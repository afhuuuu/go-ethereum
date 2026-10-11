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

package eth

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

var (
	simulateSender = common.HexToAddress("0x1000000000000000000000000000000000000001")
	simulateStore  = common.HexToAddress("0x2000000000000000000000000000000000000002")
	// Nonempty calldata writes slot 0, then both branches return its value.
	simulateStoreCode = common.FromHex("0x3615600b576000356000555b60005460005260206000f3")
)

func newSimulationRPC(t *testing.T, configure func(*core.Genesis)) (*rpc.Client, *EthAPIBackend) {
	t.Helper()
	cfg := *params.TestChainConfig
	genesis := &core.Genesis{Config: &cfg, GasLimit: 30000000, BaseFee: big.NewInt(0), Difficulty: big.NewInt(0), Alloc: types.GenesisAlloc{
		simulateSender: {Balance: new(big.Int).Mul(big.NewInt(100), big.NewInt(params.Ether))},
		simulateStore:  {Balance: big.NewInt(0), Code: simulateStoreCode, Storage: map[common.Hash]common.Hash{{}: common.BigToHash(big.NewInt(7))}},
	}}
	if configure != nil {
		configure(genesis)
	}
	db := rawdb.NewMemoryDatabase()
	engine := beacon.New(ethash.NewFaker())
	chain, err := core.NewBlockChain(db, genesis, engine, nil)
	require.NoError(t, err)
	backend := &EthAPIBackend{eth: &Ethereum{blockchain: chain, engine: engine, config: &ethconfig.Config{RPCGasCap: 10000000, RPCEVMTimeout: 5 * time.Second}}}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", ethapi.NewBlockChainAPI(backend)))
	client := rpc.DialInProc(server)
	t.Cleanup(func() { client.Close(); server.Stop(); chain.Stop(); db.Close() })
	return client, backend
}

func simulationTrace(t *testing.T, call any) map[string]any {
	t.Helper()
	trace, ok := call.(map[string]any)["trace"].(map[string]any)
	require.True(t, ok)
	return trace
}

func simulationAccount(t *testing.T, trace map[string]any, side string, addr common.Address) map[string]any {
	t.Helper()
	account, ok := trace[side].(map[string]any)[addr.Hex()].(map[string]any)
	if !ok {
		account, ok = trace[side].(map[string]any)[fmt.Sprintf("%#x", addr)].(map[string]any)
	}
	require.True(t, ok, "missing %s %s in %v", side, addr, trace)
	return account
}

func TestSimulateTraceStateSequence(t *testing.T) {
	client, backend := newSimulationRPC(t, nil)
	for _, crossBlock := range []bool{false, true} {
		t.Run(fmt.Sprint(crossBlock), func(t *testing.T) {
			values := []int{42, 43, 100}
			request := simulateRequest(simulateCall(&values[0]), simulateCall(&values[1]), simulateCall(&values[2]))
			if crossBlock {
				blocks := []any{}
				for i := range values {
					blocks = append(blocks, map[string]any{"calls": []any{simulateCall(&values[i])}})
				}
				request["blockStateCalls"] = blocks
			}
			request["tracer"] = "prestateTracer"
			request["tracerConfig"] = map[string]any{"diffMode": true}
			result := simulateRPC(t, client, request)
			var calls []any
			for _, block := range result {
				calls = append(calls, simulateCalls(block)...)
			}
			for i, call := range calls {
				preValue := 7
				if i > 0 {
					preValue = values[i-1]
				}
				trace := simulationTrace(t, call)
				if i == 1 {
					encoded, err := json.Marshal(trace)
					require.NoError(t, err)
					t.Logf("B trace, crossBlock=%v: %s", crossBlock, encoded)
				}
				pre := simulationAccount(t, trace, "pre", simulateStore)
				post := simulationAccount(t, trace, "post", simulateStore)
				require.Equal(t, fmt.Sprintf("0x%064x", preValue), pre["storage"].(map[string]any)[common.Hash{}.Hex()])
				require.Equal(t, fmt.Sprintf("0x%064x", values[i]), post["storage"].(map[string]any)[common.Hash{}.Hex()])
				require.Equal(t, float64(i+1), simulationAccount(t, trace, "post", simulateSender)["nonce"])
				if i > 0 {
					require.Equal(t, float64(i), simulationAccount(t, trace, "pre", simulateSender)["nonce"])
				}
				require.NotContains(t, post, "code", "unchanged code must be omitted from diff post")
			}
		})
	}
	// Both same-block and cross-block B read A's writes; independent B sees 7.
	n := 42
	for _, cross := range []bool{false, true} {
		request := simulateRequest(simulateCall(&n), simulateCall(nil))
		if cross {
			request["blockStateCalls"] = []any{map[string]any{"calls": []any{simulateCall(&n)}}, map[string]any{"calls": []any{simulateCall(nil)}}}
		}
		request["tracer"] = "callTracer"
		blocks := simulateRPC(t, client, request)
		last := simulateCalls(blocks[len(blocks)-1])
		require.Equal(t, fmt.Sprintf("0x%064x", 42), last[len(last)-1].(map[string]any)["returnData"])
	}
	request := simulateRequest(simulateCall(nil))
	request["tracer"] = "callTracer"
	require.Equal(t, fmt.Sprintf("0x%064x", 7), simulateCalls(simulateRPC(t, client, request)[0])[0].(map[string]any)["returnData"])
	state, _, err := backend.StateAndHeaderByNumber(context.Background(), rpc.LatestBlockNumber)
	require.NoError(t, err)
	require.Equal(t, common.BigToHash(big.NewInt(7)), state.GetState(simulateStore, common.Hash{}))
	require.Zero(t, state.GetNonce(simulateSender))
}

func TestSimulateTraceMuxEquivalence(t *testing.T) {
	client, _ := newSimulationRPC(t, nil)
	n := 42
	config := map[string]any{"callTracer": map[string]any{"withLog": true}, "prestateTracer": map[string]any{"diffMode": true}}
	request := simulateRequest(simulateCall(&n), simulateCall(nil))
	request["tracer"] = "muxTracer"
	request["tracerConfig"] = config
	mux := simulateCalls(simulateRPC(t, client, request)[0])
	for name, options := range config {
		request["tracer"] = name
		request["tracerConfig"] = options
		single := simulateCalls(simulateRPC(t, client, request)[0])
		for i := range single {
			require.Equal(t, simulationTrace(t, single[i]), simulationTrace(t, mux[i])[name])
		}
	}
	// An unchanged storage read has no fabricated zero-valued post entry.
	diff := simulationTrace(t, mux[1])["prestateTracer"].(map[string]any)
	require.NotContains(t, diff["post"], simulateStore.Hex())
}

func TestSimulateTraceOverridesAndWithdrawals(t *testing.T) {
	client, _ := newSimulationRPC(t, simulationFork)
	call := simulateCall(nil)
	request := map[string]any{"tracer": "muxTracer", "tracerConfig": map[string]any{"callTracer": nil, "prestateTracer": nil}, "blockStateCalls": []any{
		map[string]any{"blockOverrides": map[string]any{"number": "0x3", "withdrawals": []any{map[string]any{"index": "0x0", "validatorIndex": "0x0", "address": simulateStore, "amount": "0x2"}}}, "stateOverrides": map[string]any{simulateStore.Hex(): map[string]any{"stateDiff": map[string]any{common.Hash{}.Hex(): fmt.Sprintf("0x%064x", 9)}}}, "calls": []any{call}},
		map[string]any{"calls": []any{simulateCall(nil)}},
	}}
	blocks := simulateRPC(t, client, request)
	require.Len(t, blocks, 4)
	require.Empty(t, simulateCalls(blocks[0]))
	require.Empty(t, simulateCalls(blocks[1]))
	for i := 2; i < 4; i++ {
		calls := simulateCalls(blocks[i])
		require.Len(t, calls, 1)
		require.Equal(t, fmt.Sprintf("0x%064x", 9), calls[0].(map[string]any)["returnData"])
		trace := simulationTrace(t, calls[0])
		require.NotContains(t, trace["callTracer"], "calls")
		pre := trace["prestateTracer"].(map[string]any)
		account := pre[simulateStore.Hex()].(map[string]any)
		require.Equal(t, fmt.Sprintf("0x%x", (i-2)*2*params.GWei), account["balance"])
		require.Equal(t, fmt.Sprintf("0x%064x", 9), account["storage"].(map[string]any)[common.Hash{}.Hex()])
		require.NotContains(t, pre, params.WithdrawalQueueAddress.Hex())
		require.NotContains(t, pre, params.ConsolidationQueueAddress.Hex())
	}
	// Diff post belongs to the call, before the block's withdrawal credits it.
	request["tracerConfig"] = map[string]any{"prestateTracer": map[string]any{"diffMode": true}}
	blocks = simulateRPC(t, client, request)
	for _, block := range blocks[2:] {
		diff := simulationTrace(t, simulateCalls(block)[0])["prestateTracer"].(map[string]any)
		require.NotContains(t, diff["post"], simulateStore.Hex())
	}
}

func simulationFork(genesis *core.Genesis) {
	cfg := *params.MergedTestChainConfig
	zero := uint64(0)
	cfg.AmsterdamTime = &zero
	genesis.Config = &cfg
	for address, account := range core.SystemContractAllocs() {
		genesis.Alloc[address] = account
	}
}

func TestSimulateTraceCompatibility(t *testing.T) {
	for _, amsterdam := range []bool{false, true} {
		t.Run(fmt.Sprint(amsterdam), func(t *testing.T) {
			var configure func(*core.Genesis)
			if amsterdam {
				configure = simulationFork
			}
			client, backend := newSimulationRPC(t, configure)
			hash := backend.CurrentHeader().Hash()
			for _, full := range []bool{false, true} {
				for _, transfers := range []bool{false, true} {
					n := 42
					call := simulateCall(&n)
					call["value"] = "0x1"
					request := simulateRequest(call)
					request["returnFullTransactions"] = full
					request["traceTransfers"] = transfers
					request["blockStateCalls"] = []any{map[string]any{"calls": []any{call}}, map[string]any{"calls": []any{simulateCall(nil)}}}
					baseline := simulateRPC(t, client, request)
					for _, name := range []any{nil, "callTracer", "prestateTracer", "muxTracer"} {
						request["tracer"] = name
						request["tracerConfig"] = nil
						if name == "muxTracer" {
							request["tracerConfig"] = map[string]any{"callTracer": map[string]any{"withLog": true}, "prestateTracer": map[string]any{"diffMode": true}}
						}
						actual := simulateRPC(t, client, request)
						for _, block := range actual {
							for _, c := range simulateCalls(block) {
								if name == nil {
									require.NotContains(t, c, "trace")
								} else {
									require.Contains(t, c, "trace")
								}
								delete(c.(map[string]any), "trace")
							}
						}
						require.Equal(t, baseline, actual, "fork %v full %v transfers %v tracer %v", amsterdam, full, transfers, name)
					}
				}
			}
			require.Equal(t, hash, backend.CurrentHeader().Hash())
		})
	}
}

func TestSimulateTraceNativeOptions(t *testing.T) {
	child := common.HexToAddress("0x3000000000000000000000000000000000000003")
	client, _ := newSimulationRPC(t, func(g *core.Genesis) {
		g.Alloc[child] = types.Account{Balance: big.NewInt(0), Code: common.FromHex("0x602a600052600160206000a100")}
		account := g.Alloc[simulateStore]
		account.Code = common.FromHex("0x6000600060006000600073" + fmt.Sprintf("%x", child) + "61fffff15000")
		g.Alloc[simulateStore] = account
	})
	for _, top := range []bool{false, true} {
		for _, logs := range []bool{false, true} {
			request := simulateRequest(simulateCall(nil))
			request["tracer"] = "callTracer"
			request["tracerConfig"] = map[string]any{"onlyTopCall": top, "withLog": logs, "futureOption": true}
			trace := simulationTrace(t, simulateCalls(simulateRPC(t, client, request)[0])[0])
			if top {
				require.NotContains(t, trace, "calls")
			} else {
				children := trace["calls"].([]any)
				require.Len(t, children, 1)
				if logs {
					require.Len(t, children[0].(map[string]any)["logs"], 1)
				} else {
					require.NotContains(t, children[0], "logs")
				}
			}
		}
	}
	client, _ = newSimulationRPC(t, nil)
	for _, disableCode := range []bool{false, true} {
		for _, disableStorage := range []bool{false, true} {
			request := simulateRequest(simulateCall(nil))
			request["tracer"] = "prestateTracer"
			request["tracerConfig"] = map[string]any{"disableCode": disableCode, "disableStorage": disableStorage}
			trace := simulationTrace(t, simulateCalls(simulateRPC(t, client, request)[0])[0])
			account := trace[simulateStore.Hex()].(map[string]any)
			_, hasCode := account["code"]
			_, hasStorage := account["storage"]
			require.Equal(t, !disableCode, hasCode)
			require.Equal(t, !disableStorage, hasStorage)
		}
	}
	for _, name := range []string{"callTracer", "prestateTracer"} {
		request := simulateRequest(simulateCall(nil))
		request["tracer"] = name
		want := simulateRPC(t, client, request)
		request["tracerConfig"] = nil
		require.Equal(t, want, simulateRPC(t, client, request))
	}
}

func TestSimulateTraceRevert(t *testing.T) {
	revertAddr := common.HexToAddress("0x3000000000000000000000000000000000000003")
	for _, top := range []bool{true, false} {
		t.Run(fmt.Sprint(top), func(t *testing.T) {
			client, _ := newSimulationRPC(t, func(g *core.Genesis) {
				body := common.FromHex("0x6063600055602a600052600160206000a160006000fd")
				code := append([]byte{0x36, 0x15, 0x60, byte(5 + len(body)), 0x57}, body...)
				code = append(code, common.FromHex("0x5b60005460005260206000f3")...)
				g.Alloc[revertAddr] = types.Account{Balance: big.NewInt(0), Storage: map[common.Hash]common.Hash{{}: common.BigToHash(big.NewInt(7))}, Code: code}
				if !top {
					a := g.Alloc[simulateStore]
					a.Code = common.FromHex("0x6000600060016000600073" + fmt.Sprintf("%x", revertAddr) + "61fffff15000")
					g.Alloc[simulateStore] = a
				}
			})
			call := simulateCall(nil)
			if top {
				call["to"] = revertAddr
				call["data"] = "0x01"
			}
			read := simulateCall(nil)
			read["to"] = revertAddr
			request := simulateRequest(call, read)
			baseline := simulateRPC(t, client, request)
			request["tracer"] = "muxTracer"
			request["tracerConfig"] = map[string]any{"callTracer": map[string]any{"withLog": true}, "prestateTracer": map[string]any{"diffMode": true}}
			actual := simulateRPC(t, client, request)
			calls := simulateCalls(actual[0])
			require.Equal(t, fmt.Sprintf("0x%064x", 7), calls[1].(map[string]any)["returnData"])
			result := calls[0].(map[string]any)
			trace := simulationTrace(t, result)
			require.Empty(t, result["logs"])
			ct := trace["callTracer"].(map[string]any)
			if top {
				require.Equal(t, "0x0", result["status"])
				require.Contains(t, ct, "error")
			} else {
				require.Equal(t, "0x1", result["status"])
				child := ct["calls"].([]any)[0].(map[string]any)
				require.Contains(t, child, "error")
				require.NotContains(t, child, "logs")
			}
			diff := trace["prestateTracer"].(map[string]any)
			require.NotContains(t, diff["post"], fmt.Sprintf("%#x", revertAddr))
			require.Equal(t, float64(1), simulationAccount(t, diff, "post", simulateSender)["nonce"])
			for _, c := range calls {
				delete(c.(map[string]any), "trace")
			}
			require.Equal(t, baseline, actual)
		})
	}
}

func TestSimulateTraceCreate(t *testing.T) {
	for _, nonce := range []string{"0x0", "0x9"} {
		t.Run(nonce, func(t *testing.T) {
			client, _ := newSimulationRPC(t, nil)
			runtime := common.FromHex("0x602a60005260206000f3")
			init := fmt.Sprintf("0x60%02x600c60003960%02x6000f3%x", len(runtime), len(runtime), runtime)
			address := crypto.CreateAddress(simulateSender, 0)
			create := map[string]any{"from": simulateSender, "gas": "0x30d40", "data": init, "nonce": nonce}
			call := simulateCall(nil)
			call["to"] = address
			request := simulateRequest(create, call)
			request["tracer"] = "muxTracer"
			request["tracerConfig"] = map[string]any{"callTracer": nil, "prestateTracer": map[string]any{"diffMode": true}}
			calls := simulateCalls(simulateRPC(t, client, request)[0])
			require.Equal(t, fmt.Sprintf("0x%064x", 42), calls[1].(map[string]any)["returnData"])
			trace := simulationTrace(t, calls[0])
			require.Equal(t, fmt.Sprintf("%#x", address), trace["callTracer"].(map[string]any)["to"])
			require.Equal(t, fmt.Sprintf("0x%x", runtime), simulationAccount(t, trace["prestateTracer"].(map[string]any), "post", address)["code"])
		})
	}
}

func TestSimulateTraceTransfers(t *testing.T) {
	client, _ := newSimulationRPC(t, func(g *core.Genesis) {
		a := g.Alloc[simulateStore]
		a.Code = common.FromHex("0x602a600052600160206000a100")
		g.Alloc[simulateStore] = a
	})
	call := simulateCall(nil)
	call["value"] = "0x1"
	request := simulateRequest(call)
	request["tracer"] = "callTracer"
	request["tracerConfig"] = map[string]any{"withLog": true}
	request["traceTransfers"] = true
	result := simulateCalls(simulateRPC(t, client, request)[0])[0].(map[string]any)
	require.Len(t, result["logs"], 2)
	require.Len(t, simulationTrace(t, result)["logs"], 1)
	require.Equal(t, fmt.Sprintf("0x%064x", 42), simulationTrace(t, result)["logs"].([]any)[0].(map[string]any)["data"])
	require.Equal(t, "0x1", simulationTrace(t, result)["value"])
}

func TestSimulateTraceFactoryHooks(t *testing.T) {
	_, backend := newSimulationRPC(t, nil)
	for _, name := range []string{"callTracer", "prestateTracer", "muxTracer"} {
		config := json.RawMessage(`{}`)
		if name == "muxTracer" {
			config = json.RawMessage(`{"callTracer":{}}`)
		}
		factory, err := backend.SimulationTracerFactory(name, config)
		require.NoError(t, err)
		for i := range config {
			config[i] = ' '
		}
		first, err := factory(ethapi.SimulationTraceContext{}, &tracing.Hooks{})
		require.NoError(t, err)
		second, err := factory(ethapi.SimulationTraceContext{}, &tracing.Hooks{})
		require.NoError(t, err)
		require.NotSame(t, first.Hooks, second.Hooks)
		require.Nil(t, first.Hooks.OnGasChangeV2)
		require.Nil(t, first.Hooks.OnFaultV2)
		require.Nil(t, first.Hooks.OnStorageChange)
		if name != "prestateTracer" {
			require.Nil(t, first.Hooks.OnOpcode)
			require.Nil(t, first.Hooks.OnOpcodeV2)
		} else {
			require.NotNil(t, first.Hooks.OnOpcodeV2)
		}
	}
}

func TestSimulateTraceConcurrent(t *testing.T) {
	client, _ := newSimulationRPC(t, nil)
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func(i int) {
			n := 40 + i
			request := simulateRequest(simulateCall(&n), simulateCall(nil))
			request["tracer"] = "muxTracer"
			request["tracerConfig"] = map[string]any{"callTracer": nil, "prestateTracer": map[string]any{"diffMode": true}}
			var result []map[string]any
			err := client.CallContext(context.Background(), &result, "eth_simulateV1", request, "latest")
			if err == nil {
				calls := simulateCalls(result[0])
				if calls[1].(map[string]any)["returnData"] != fmt.Sprintf("0x%064x", n) {
					err = fmt.Errorf("request %d inherited another request's state", i)
				}
			}
			errors <- err
		}(i)
	}
	for i := 0; i < 12; i++ {
		require.NoError(t, <-errors)
	}
}

func simulateCall(value *int) map[string]any {
	call := map[string]any{"from": simulateSender, "to": simulateStore, "gas": "0x186a0"}
	if value != nil {
		call["data"] = fmt.Sprintf("0x%064x", *value)
	}
	return call
}

func simulateRequest(calls ...map[string]any) map[string]any {
	return map[string]any{"blockStateCalls": []any{map[string]any{"calls": calls}}}
}

func simulateRPC(t *testing.T, client *rpc.Client, request map[string]any) []map[string]any {
	t.Helper()
	var result []map[string]any
	require.NoError(t, client.CallContext(context.Background(), &result, "eth_simulateV1", request, "latest"))
	return result
}

func simulateCalls(block map[string]any) []any { return block["calls"].([]any) }

func TestSimulateTraceRPC(t *testing.T) {
	client, _ := newSimulationRPC(t, nil)
	n := 42
	for _, name := range []string{"callTracer", "prestateTracer", "muxTracer"} {
		t.Run(name, func(t *testing.T) {
			request := simulateRequest(simulateCall(&n), simulateCall(nil))
			request["tracer"] = name
			if name == "muxTracer" {
				request["tracerConfig"] = map[string]any{"callTracer": map[string]any{}, "prestateTracer": map[string]any{"diffMode": true}}
			}
			result := simulateRPC(t, client, request)
			calls := simulateCalls(result[0])
			require.Equal(t, fmt.Sprintf("0x%064x", 42), calls[1].(map[string]any)["returnData"])
			for _, call := range calls {
				trace, ok := call.(map[string]any)["trace"].(map[string]any)
				require.True(t, ok, "native trace object missing")
				require.NotEmpty(t, trace)
			}
		})
	}
}

func TestSimulateTraceInvalidConfig(t *testing.T) {
	client, _ := newSimulationRPC(t, nil)
	for _, fields := range []string{
		`{"tracer":""}`, `{"tracer":"  "}`, `{"tracer":"unknown"}`, `{"tracer":"function(){}"}`, `{"tracer":"4byteTracer"}`,
		`{"tracerConfig":{}}`, `{"tracer":null,"tracerConfig":{}}`, `{"tracer":"callTracer","tracerConfig":[]}`,
		`{"tracer":"callTracer","tracerConfig":{"withLog":"yes"}}`, `{"tracer":"prestateTracer","tracerConfig":{"diffMode":true,"includeEmpty":true}}`,
		`{"tracer":"muxTracer"}`, `{"tracer":"muxTracer","tracerConfig":null}`, `{"tracer":"muxTracer","tracerConfig":{}}`,
		`{"tracer":"muxTracer","tracerConfig":{"unknown":{}}}`, `{"tracer":"muxTracer","tracerConfig":{"muxTracer":{}}}`,
		`{"tracer":"muxTracer","tracerConfig":{"callTracer":[]}}`, `{"tracer":"prestateTracer","tracerConfig":1}`,
	} {
		t.Run(fields, func(t *testing.T) {
			var request map[string]any
			require.NoError(t, json.Unmarshal([]byte(fields), &request))
			request["blockStateCalls"] = []any{map[string]any{"calls": []any{}}}
			var result any
			err := client.CallContext(context.Background(), &result, "eth_simulateV1", request, "latest")
			require.Error(t, err, "invalid configuration was accepted")
			rpcErr, ok := err.(rpc.Error)
			require.True(t, ok)
			require.Equal(t, -32602, rpcErr.ErrorCode())
			require.Nil(t, result)
		})
	}
}

func TestSimulateTraceWithoutReceiptExport(t *testing.T) {
	client, _ := newSimulationRPC(t, nil)
	for _, tracerName := range []string{"none", "callTracer", "prestateTracer", "muxTracer"} {
		for _, receiptOption := range []string{"omitted", "false", "true"} {
			t.Run(tracerName+"/"+receiptOption, func(t *testing.T) {
				n := 42
				request := simulateRequest(simulateCall(&n), simulateCall(nil))
				switch tracerName {
				case "callTracer":
					request["tracer"] = tracerName
					request["tracerConfig"] = map[string]any{"withLog": true}
				case "prestateTracer":
					request["tracer"] = tracerName
					request["tracerConfig"] = map[string]any{"diffMode": true}
				case "muxTracer":
					request["tracer"] = tracerName
					request["tracerConfig"] = map[string]any{
						"callTracer":     map[string]any{"withLog": true},
						"prestateTracer": map[string]any{"diffMode": true},
					}
				}
				if receiptOption != "omitted" {
					request["returnReceipts"] = receiptOption == "true"
				}
				blocks := simulateRPC(t, client, request)
				require.Len(t, blocks, 1)
				calls := simulateCalls(blocks[0])
				require.Len(t, calls, 2)
				require.Equal(t, fmt.Sprintf("0x%064x", 42), calls[1].(map[string]any)["returnData"])
				for _, call := range calls {
					fields := call.(map[string]any)
					require.NotContains(t, fields, "receipt")
					if tracerName == "none" {
						require.NotContains(t, fields, "trace")
					} else {
						require.NotEmpty(t, simulationTrace(t, call))
					}
				}
				if tracerName == "muxTracer" && receiptOption == "omitted" {
					example, err := json.Marshal(map[string]any{
						"request": map[string]any{
							"jsonrpc": "2.0", "id": 1, "method": "eth_simulateV1",
							"params": []any{request, "latest"},
						},
						"result": blocks,
					})
					require.NoError(t, err)
					t.Logf("trace-only local fixture example: %s", example)
				}
			})
		}
	}
}
