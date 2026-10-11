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
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/eth/tracers"
	"github.com/ethereum/go-ethereum/eth/tracers/native"
	"github.com/ethereum/go-ethereum/internal/ethapi"
)

// SimulationTracerFactory validates configuration once, then constructs a fresh
// native tracer for every call without introducing ethapi -> tracers imports.
func (b *EthAPIBackend) SimulationTracerFactory(name string, config json.RawMessage) (ethapi.SimulationTracerFactory, error) {
	config = append(json.RawMessage(nil), config...)
	if err := validateSimulationTracer(name, config, false); err != nil {
		return nil, err
	}
	create := func(ctx *tracers.Context) (*tracers.Tracer, error) {
		if name != "muxTracer" {
			return tracers.DefaultDirectory.New(name, ctx, config, b.ChainConfig())
		}
		var configs map[string]json.RawMessage
		if err := json.Unmarshal(config, &configs); err != nil {
			return nil, err
		}
		names := make([]string, 0, len(configs))
		for name := range configs {
			names = append(names, name)
		}
		sort.Strings(names)
		children := make([]*tracers.Tracer, 0, len(names))
		for _, name := range names {
			child, err := tracers.DefaultDirectory.New(name, ctx, configs[name], b.ChainConfig())
			if err != nil {
				return nil, err
			}
			children = append(children, child)
		}
		return simulationMux(names, children)
	}
	// Native constructors preserve their own option decoding and conflicts, even
	// when a request contains only empty blocks and no transaction is executed.
	if _, err := create(&tracers.Context{}); err != nil {
		return nil, err
	}
	return func(ctx ethapi.SimulationTraceContext, legacy *tracing.Hooks) (*ethapi.SimulationTracer, error) {
		nativeCtx := &tracers.Context{BlockHash: ctx.BlockHash, TxIndex: ctx.TxIndex, TxHash: ctx.TxHash}
		if ctx.BlockNumber != nil {
			nativeCtx.BlockNumber = new(big.Int).Set(ctx.BlockNumber)
		}
		user, err := create(nativeCtx)
		if err != nil {
			return nil, err
		}
		combined, err := simulationMux([]string{"legacy", "user"}, []*tracers.Tracer{{Hooks: legacy}, user})
		if err != nil {
			return nil, err
		}
		return &ethapi.SimulationTracer{Hooks: combined.Hooks, GetResult: user.GetResult, Stop: user.Stop}, nil
	}, nil
}

func validateSimulationTracer(name string, config json.RawMessage, child bool) error {
	if name != "callTracer" && name != "prestateTracer" && (name != "muxTracer" || child) {
		return fmt.Errorf("unsupported simulation tracer: %s", name)
	}
	config = bytes.TrimSpace(config)
	if len(config) == 0 || bytes.Equal(config, []byte("null")) {
		if name == "muxTracer" {
			return fmt.Errorf("muxTracer requires a nonempty configuration object")
		}
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(config, &object); err != nil {
		return fmt.Errorf("%s configuration must be an object: %w", name, err)
	}
	if name == "muxTracer" {
		if len(object) == 0 {
			return fmt.Errorf("muxTracer requires a nonempty configuration object")
		}
		for childName, childConfig := range object {
			if err := validateSimulationTracer(childName, childConfig, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// simulationMux delegates V2/V1 dispatch to the native mux, but removes hooks
// no child consumes. In particular call-only tracing must not observe opcodes.
func simulationMux(names []string, children []*tracers.Tracer) (*tracers.Tracer, error) {
	mux, err := native.NewMuxTracer(names, children)
	if err != nil {
		return nil, err
	}
	var start, end, enter, exit, opcode, fault, gas, balance, nonce, code, storage, log, systemStart, systemEnd bool
	for _, child := range children {
		h := child.Hooks
		start = start || h.OnTxStart != nil
		end = end || h.OnTxEnd != nil
		enter = enter || h.OnEnter != nil || h.OnEnterV2 != nil
		exit = exit || h.OnExit != nil || h.OnExitV2 != nil
		opcode = opcode || h.OnOpcode != nil || h.OnOpcodeV2 != nil
		fault = fault || h.OnFault != nil || h.OnFaultV2 != nil
		gas = gas || h.OnGasChange != nil || h.OnGasChangeV2 != nil
		balance = balance || h.OnBalanceChange != nil
		nonce = nonce || h.OnNonceChange != nil || h.OnNonceChangeV2 != nil
		code = code || h.OnCodeChange != nil || h.OnCodeChangeV2 != nil
		storage = storage || h.OnStorageChange != nil
		log = log || h.OnLog != nil
		systemStart = systemStart || h.OnSystemCallStart != nil || h.OnSystemCallStartV2 != nil
		systemEnd = systemEnd || h.OnSystemCallEnd != nil
	}
	if !start {
		mux.OnTxStart = nil
	}
	if !end {
		mux.OnTxEnd = nil
	}
	if !enter {
		mux.OnEnterV2 = nil
	}
	if !exit {
		mux.OnExitV2 = nil
	}
	if !opcode {
		mux.OnOpcodeV2 = nil
	}
	if !fault {
		mux.OnFaultV2 = nil
	}
	if !gas {
		mux.OnGasChangeV2 = nil
	}
	if !balance {
		mux.OnBalanceChange = nil
	}
	if !nonce {
		mux.OnNonceChangeV2 = nil
	}
	if !code {
		mux.OnCodeChangeV2 = nil
	}
	if !storage {
		mux.OnStorageChange = nil
	}
	if !log {
		mux.OnLog = nil
	}
	if !systemStart {
		mux.OnSystemCallStartV2 = nil
	}
	if !systemEnd {
		mux.OnSystemCallEnd = nil
	}
	return mux, nil
}
