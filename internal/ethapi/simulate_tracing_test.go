// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package ethapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/internal/ethapi/override"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

type simulationTimeoutBackend struct {
	*testBackend
	timeout time.Duration
}

type nilSimulationFactoryBackend struct{ Backend }

func (nilSimulationFactoryBackend) SimulationTracerFactory(string, json.RawMessage) (SimulationTracerFactory, error) {
	return nil, nil
}

func TestSimulateTraceFactoryContract(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprint("empty=", empty), func(t *testing.T) {
			sim, blocks := newTraceLifecycleSimulation(t, []byte{0})
			if empty {
				blocks[0].Calls = nil
			}
			api := NewBlockChainAPI(nilSimulationFactoryBackend{sim.b})
			name := "callTracer"
			result, err := api.SimulateV1(context.Background(), simOpts{BlockStateCalls: blocks, Tracer: &name}, nil)
			require.Error(t, err)
			require.Nil(t, result)
			require.ErrorContains(t, err, "factory")
			var rpcErr rpc.Error
			if errors.As(err, &rpcErr) {
				require.NotEqual(t, -32602, rpcErr.ErrorCode())
			}
			result, err = api.SimulateV1(context.Background(), simOpts{BlockStateCalls: blocks}, nil)
			require.NoError(t, err)
			require.Len(t, result, 1)
			for _, call := range result[0].Calls {
				require.Nil(t, call.Trace)
			}
		})
	}
}

func TestSimulateTraceResultContract(t *testing.T) {
	sentinel := errors.New("getter failed")
	for _, tc := range []struct {
		name      string
		raw       json.RawMessage
		getterErr error
		valid     bool
	}{
		{"nil", nil, nil, false},
		{"empty", json.RawMessage{}, nil, false},
		{"whitespace", json.RawMessage(" \n\t "), nil, false},
		{"unfinished", json.RawMessage(`{"x":`), nil, false},
		{"malformed", json.RawMessage(`{"x":}`), nil, false},
		{"null", json.RawMessage(`null`), nil, false},
		{"padded-null", json.RawMessage(" \nnull\t "), nil, false},
		{"empty-object", json.RawMessage(`{}`), nil, true},
		{"object", json.RawMessage(" \n{\"x\": 1}\t "), nil, true},
		{"getter-error", json.RawMessage(`{"x":1}`), sentinel, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sim, blocks := newTraceLifecycleSimulation(t, []byte{0})
			blocks[0].Calls = append(blocks[0].Calls, blocks[0].Calls[0], blocks[0].Calls[0])
			var starts, ends, getters, factories int
			var stops atomic.Int32
			sim.traceFactory = func(_ SimulationTraceContext, legacy *tracing.Hooks) (*SimulationTracer, error) {
				factories++
				index := factories
				hooks := *legacy
				hooks.OnTxStart = func(*tracing.VMContext, *types.Transaction, common.Address) {
					starts++
					require.Equal(t, index, starts)
				}
				hooks.OnTxEnd = func(receipt *types.Receipt, err error) {
					ends++
					require.Equal(t, index, ends)
					require.NotNil(t, receipt)
					require.NoError(t, err)
				}
				return &SimulationTracer{Hooks: &hooks, GetResult: func() (json.RawMessage, error) {
					getters++
					require.Equal(t, index, getters)
					require.Equal(t, getters, ends)
					if index == 1 {
						return json.RawMessage(`{"first":true}`), nil
					}
					return tc.raw, tc.getterErr
				}, Stop: func(err error) {
					if err == nil {
						panic("Stop(nil)")
					}
					stops.Add(1)
				}}, nil
			}
			result, err := sim.execute(context.Background(), blocks)
			want := 2
			if tc.valid {
				want = 3
				require.NoError(t, err)
				require.Equal(t, json.RawMessage(`{"first":true}`), result[0].Calls[0].Trace)
				for _, call := range result[0].Calls[1:] {
					require.Equal(t, tc.raw, call.Trace)
				}
			} else {
				require.Error(t, err)
				require.Nil(t, result, "must not export the earlier successful call")
				if tc.getterErr != nil {
					require.ErrorIs(t, err, sentinel)
				}
			}
			require.Equal(t, want, factories)
			require.Equal(t, want, starts)
			require.Equal(t, want, ends)
			require.Equal(t, want, getters)
			require.Zero(t, stops.Load(), "completed observers must be joined before request cancellation")
		})
	}
}

func (b *simulationTimeoutBackend) RPCEVMTimeout() time.Duration { return b.timeout }

func newTraceLifecycleSimulation(t *testing.T, code []byte, configure ...func(*core.Genesis)) (*simulator, []simBlock) {
	t.Helper()
	sender := common.HexToAddress("0x1000000000000000000000000000000000000001")
	to := common.HexToAddress("0x2000000000000000000000000000000000000002")
	cfg := *params.TestChainConfig
	genesis := &core.Genesis{Config: &cfg, GasLimit: 30000000, Difficulty: big.NewInt(0), BaseFee: big.NewInt(0), Alloc: types.GenesisAlloc{
		sender: {Balance: big.NewInt(params.Ether)}, to: {Balance: big.NewInt(0), Code: code},
	}}
	for _, f := range configure {
		f(genesis)
	}
	db := rawdb.NewMemoryDatabase()
	engine := beacon.New(ethash.NewFaker())
	chain, err := core.NewBlockChain(db, genesis, engine, nil)
	require.NoError(t, err)
	t.Cleanup(func() { chain.Stop(); db.Close() })
	backend := &simulationTimeoutBackend{testBackend: &testBackend{db: db, chain: chain}, timeout: 0}
	state, header, err := backend.StateAndHeaderByNumberOrHash(context.Background(), rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber))
	require.NoError(t, err)
	sim := &simulator{b: backend, state: state, base: header, chainConfig: genesis.Config, budget: newGasBudget(10000000)}
	gas := hexutil.Uint64(100000)
	return sim, []simBlock{{Calls: []TransactionArgs{{From: &sender, To: &to, Gas: &gas}}}}
}

func TestSimulateTraceSystemBoundaries(t *testing.T) {
	sim, blocks := newTraceLifecycleSimulation(t, []byte{0}, func(g *core.Genesis) {
		cfg := *params.MergedTestChainConfig
		g.Config = &cfg
		for addr, account := range core.SystemContractAllocs() {
			g.Alloc[addr] = account
		}
	})
	to := *blocks[0].Calls[0].To
	number := hexutil.Big(*big.NewInt(3))
	withdrawals := types.Withdrawals{&types.Withdrawal{Address: to, Amount: 2}}
	blocks[0].BlockOverrides = &override.BlockOverrides{Number: &number, Withdrawals: &withdrawals}
	blocks = append(blocks, simBlock{Calls: append([]TransactionArgs(nil), blocks[0].Calls...)})
	starts, ends, systems, balanceChanges := 0, 0, 0, 0
	sim.traceFactory = func(ctx SimulationTraceContext, legacy *tracing.Hooks) (*SimulationTracer, error) {
		require.Equal(t, int64(3+starts), ctx.BlockNumber.Int64())
		hooks := *legacy
		hooks.OnTxStart = func(env *tracing.VMContext, _ *types.Transaction, _ common.Address) {
			starts++
			require.Equal(t, uint64((starts-1)*2*params.GWei), env.StateDB.GetBalance(to).Uint64())
		}
		hooks.OnTxEnd = func(*types.Receipt, error) {
			ends++
			require.Equal(t, uint64((ends-1)*2*params.GWei), sim.state.GetBalance(to).Uint64())
		}
		hooks.OnSystemCallStartV2 = func(*tracing.VMContext) { systems++ }
		hooks.OnSystemCallEnd = func() { systems++ }
		hooks.OnBalanceChange = func(_ common.Address, _, _ *big.Int, reason tracing.BalanceChangeReason) {
			if reason == tracing.BalanceIncreaseWithdrawal {
				balanceChanges++
			}
		}
		return &SimulationTracer{Hooks: &hooks, GetResult: func() (json.RawMessage, error) { return json.RawMessage(`{}`), nil }, Stop: func(error) { t.Error("unexpected stop") }}, nil
	}
	result, err := sim.execute(context.Background(), blocks)
	require.NoError(t, err)
	require.Len(t, result, 4)
	require.Empty(t, result[0].Calls)
	require.Empty(t, result[1].Calls)
	require.Equal(t, 2, starts)
	require.Equal(t, 2, ends)
	require.Zero(t, systems)
	require.Zero(t, balanceChanges)
	require.Equal(t, uint256.NewInt(2*params.GWei), sim.state.GetBalance(to))
}

func TestSimulateTraceBackendCapability(t *testing.T) {
	sim, blocks := newTraceLifecycleSimulation(t, []byte{0})
	api := NewBlockChainAPI(sim.b)
	_, err := api.SimulateV1(context.Background(), simOpts{BlockStateCalls: blocks}, nil)
	require.NoError(t, err)
	for _, name := range []string{"callTracer", "prestateTracer", "muxTracer"} {
		_, err := api.SimulateV1(context.Background(), simOpts{BlockStateCalls: blocks, Tracer: &name}, nil)
		require.ErrorContains(t, err, "backend does not support simulation tracing")
	}
}

func TestSimulateTraceLifecycle(t *testing.T) {
	for _, scenario := range []string{"success", "revert", "validation", "budget", "factory", "getResult"} {
		t.Run(scenario, func(t *testing.T) {
			code := common.FromHex("0x600160005500")
			if scenario == "revert" {
				code = common.FromHex("0x600160005560006000fd")
			}
			sim, blocks := newTraceLifecycleSimulation(t, code)
			if scenario == "validation" {
				sim.validate = true
				nonce := hexutil.Uint64(9)
				blocks[0].Calls[0].Nonce = &nonce
			}
			starts, ends, getters, storage := 0, 0, 0, 0
			var stops atomic.Int32
			sentinel := errors.New("test tracer failure")
			sim.traceFactory = func(ctx SimulationTraceContext, legacy *tracing.Hooks) (*SimulationTracer, error) {
				require.Equal(t, big.NewInt(1), ctx.BlockNumber)
				require.Zero(t, ctx.TxIndex)
				require.NotEqual(t, common.Hash{}, ctx.TxHash)
				require.Equal(t, common.Hash{}, ctx.BlockHash)
				// Mutating the factory context must not mutate the simulated header.
				ctx.BlockNumber.SetInt64(999)
				require.Nil(t, legacy.OnTxStart)
				require.Nil(t, legacy.OnTxEnd)
				if scenario == "factory" {
					return nil, sentinel
				}
				hooks := *legacy
				hooks.OnTxStart = func(env *tracing.VMContext, tx *types.Transaction, from common.Address) {
					starts++
					require.Zero(t, env.StateDB.GetNonce(from))
					if scenario == "budget" {
						sim.budget.remaining = 0
					}
				}
				hooks.OnStorageChange = func(common.Address, common.Hash, common.Hash, common.Hash) { storage++ }
				hooks.OnTxEnd = func(receipt *types.Receipt, err error) {
					ends++
					if scenario == "validation" {
						require.Nil(t, receipt)
						require.Error(t, err)
						return
					}
					require.NotNil(t, receipt)
					if scenario == "budget" {
						require.ErrorContains(t, err, "RPC gas cap exhausted")
					} else {
						require.NoError(t, err)
					}
					require.Positive(t, receipt.GasUsed)
					want := types.ReceiptStatusSuccessful
					if scenario == "revert" {
						want = types.ReceiptStatusFailed
					}
					require.Equal(t, want, receipt.Status)
					require.Equal(t, big.NewInt(1), receipt.BlockNumber)
				}
				return &SimulationTracer{Hooks: &hooks, GetResult: func() (json.RawMessage, error) {
					getters++
					require.Equal(t, 1, ends)
					if scenario == "getResult" {
						return nil, sentinel
					}
					return json.RawMessage(`{"ok":true}`), nil
				}, Stop: func(err error) {
					if err == nil {
						panic("Stop(nil)")
					}
					stops.Add(1)
				}}, nil
			}
			result, err := sim.execute(context.Background(), blocks)
			if scenario == "success" || scenario == "revert" {
				require.NoError(t, err)
				require.Len(t, result, 1)
				require.JSONEq(t, `{"ok":true}`, string(result[0].Calls[0].Trace))
			} else {
				require.Error(t, err)
				require.Nil(t, result)
			}
			if scenario == "factory" {
				require.Zero(t, starts)
				require.Zero(t, ends)
			} else {
				require.Equal(t, 1, starts)
				require.Equal(t, 1, ends)
			}
			if scenario == "success" || scenario == "revert" || scenario == "getResult" {
				require.Equal(t, 1, getters)
				require.Positive(t, storage)
			} else {
				require.Zero(t, getters)
			}
			require.Zero(t, stops.Load(), "observer leaked past transaction/request completion")
		})
	}
}

func TestSimulateTraceCancel(t *testing.T) {
	for _, phase := range []string{"execution", "getResult", "timeout"} {
		t.Run(phase, func(t *testing.T) {
			sim, blocks := newTraceLifecycleSimulation(t, common.FromHex("0x600160005500"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if phase == "timeout" {
				sim.b.(*simulationTimeoutBackend).timeout = 50 * time.Millisecond
			}
			entered, stopped := make(chan struct{}), make(chan struct{})
			var starts, ends, stopCount atomic.Int32
			sim.traceFactory = func(_ SimulationTraceContext, legacy *tracing.Hooks) (*SimulationTracer, error) {
				hooks := *legacy
				hooks.OnTxStart = func(*tracing.VMContext, *types.Transaction, common.Address) { starts.Add(1) }
				hooks.OnTxEnd = func(*types.Receipt, error) { ends.Add(1) }
				if phase == "execution" {
					hooks.OnOpcode = func(uint64, byte, uint64, uint64, tracing.OpContext, []byte, int, error) {
						if starts.CompareAndSwap(1, 2) {
							close(entered)
							<-stopped
						}
					}
				}
				return &SimulationTracer{Hooks: &hooks, GetResult: func() (json.RawMessage, error) {
					if phase != "execution" {
						close(entered)
						<-stopped
					}
					return json.RawMessage(`{}`), nil
				}, Stop: func(err error) {
					if err == nil {
						panic("Stop(nil)")
					}
					if stopCount.Add(1) == 1 {
						close(stopped)
					}
				}}, nil
			}
			finished := make(chan error, 1)
			go func() { _, err := sim.execute(ctx, blocks); finished <- err }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("execution/getResult not reached")
			}
			if phase != "timeout" {
				cancel()
			}
			select {
			case err := <-finished:
				require.Error(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("simulation cancellation leaked")
			}
			require.Equal(t, int32(1), stopCount.Load())
			require.Equal(t, int32(1), ends.Load())
		})
	}
}

func TestSimulateTraceObserverIsolation(t *testing.T) {
	sim, blocks := newTraceLifecycleSimulation(t, []byte{byte(vm.STOP)})
	blocks[0].Calls = append(blocks[0].Calls, blocks[0].Calls[0])
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstStops, secondStops := atomic.Int32{}, atomic.Int32{}
	entered, stopped := make(chan struct{}), make(chan struct{})
	count := 0
	sim.traceFactory = func(_ SimulationTraceContext, legacy *tracing.Hooks) (*SimulationTracer, error) {
		count++
		index := count
		hooks := *legacy
		return &SimulationTracer{Hooks: &hooks, GetResult: func() (json.RawMessage, error) {
			if index == 2 {
				close(entered)
				<-stopped
			}
			return json.RawMessage(`{}`), nil
		}, Stop: func(err error) {
			if err == nil {
				panic("Stop(nil)")
			}
			if index == 1 {
				firstStops.Add(1)
			} else {
				secondStops.Add(1)
				close(stopped)
			}
		}}, nil
	}
	finished := make(chan error, 1)
	go func() { _, err := sim.execute(ctx, blocks); finished <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("second tracer not reached")
	}
	cancel()
	select {
	case err := <-finished:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("observer not joined")
	}
	require.Zero(t, firstStops.Load())
	require.Equal(t, int32(1), secondStops.Load())
}

func TestSimulateTraceSingleExecution(t *testing.T) {
	sim, blocks := newTraceLifecycleSimulation(t, common.FromHex("0x600160005500"))
	starts, ends, writes, getters := 0, 0, 0, 0
	sim.traceFactory = func(_ SimulationTraceContext, legacy *tracing.Hooks) (*SimulationTracer, error) {
		hooks := *legacy
		hooks.OnTxStart = func(*tracing.VMContext, *types.Transaction, common.Address) { starts++ }
		hooks.OnTxEnd = func(*types.Receipt, error) { ends++ }
		hooks.OnStorageChange = func(common.Address, common.Hash, common.Hash, common.Hash) { writes++ }
		return &SimulationTracer{
			Hooks: &hooks,
			GetResult: func() (json.RawMessage, error) {
				getters++
				return json.RawMessage(`{}`), nil
			},
			Stop: func(error) { t.Error("unexpected Stop") },
		}, nil
	}
	result, err := sim.execute(context.Background(), blocks)
	require.NoError(t, err)
	require.Equal(t, 1, starts)
	require.Equal(t, 1, ends)
	require.Equal(t, 1, writes)
	require.Equal(t, 1, getters)
	require.Len(t, result, 1)
	require.Len(t, result[0].Calls, 1)
	require.JSONEq(t, "{}", string(result[0].Calls[0].Trace))
	require.Equal(t, uint64(1), sim.state.GetNonce(*blocks[0].Calls[0].From))
}
