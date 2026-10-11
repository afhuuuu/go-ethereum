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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
)

// SimulationTraceContext describes a transaction before its simulated block is
// assembled. BlockHash is zero because the final block hash is not yet known.
type SimulationTraceContext struct {
	BlockHash   common.Hash
	BlockNumber *big.Int
	TxIndex     int
	TxHash      common.Hash
}

// SimulationTracer owns one transaction's hooks and user-visible result.
// Stop must be safe to invoke concurrently with execution and GetResult.
type SimulationTracer struct {
	Hooks     *tracing.Hooks
	GetResult func() (json.RawMessage, error)
	Stop      func(error)
}

// SimulationTracerFactory creates a fresh tracer, combining it with legacyHooks.
// The factory must not mutate or retain mutable aliases of its context fields.
type SimulationTracerFactory func(SimulationTraceContext, *tracing.Hooks) (*SimulationTracer, error)

type simulationTracerProvider interface {
	SimulationTracerFactory(string, json.RawMessage) (SimulationTracerFactory, error)
}

func simulationTraceFactory(b Backend, name *string, config json.RawMessage) (SimulationTracerFactory, error) {
	if name == nil {
		if len(config) != 0 && !bytes.Equal(bytes.TrimSpace(config), []byte("null")) {
			return nil, &invalidParamsError{message: "tracerConfig requires tracer"}
		}
		return nil, nil
	}
	switch *name {
	case "callTracer", "prestateTracer", "muxTracer":
	default:
		return nil, &invalidParamsError{message: "unsupported simulation tracer: " + *name}
	}
	provider, ok := b.(simulationTracerProvider)
	if !ok {
		return nil, errors.New("backend does not support simulation tracing")
	}
	factory, err := provider.SimulationTracerFactory(*name, config)
	if err != nil {
		return nil, &invalidParamsError{message: err.Error()}
	}
	if factory == nil {
		return nil, errors.New("backend returned a nil simulation tracer factory")
	}
	return factory, nil
}

// simulationTraceSession keeps the observer alive through GetResult, then joins
// it before restoring hooks. Only the execution goroutine touches hooks/state.
type simulationTraceSession struct {
	tracer       *SimulationTracer
	live, legacy *tracing.Hooks
	done, joined chan struct{}
}

func startSimulationTrace(ctx context.Context, factory SimulationTracerFactory, traceContext SimulationTraceContext, legacy, live *tracing.Hooks, evm *vm.EVM, tx *types.Transaction, from common.Address) (*simulationTraceSession, error) {
	tracer, err := factory(traceContext, legacy)
	if err != nil {
		return nil, err
	}
	if tracer == nil || tracer.Hooks == nil || tracer.GetResult == nil || tracer.Stop == nil {
		return nil, errors.New("invalid simulation tracer factory result")
	}
	session := &simulationTraceSession{tracer: tracer, live: live, legacy: legacy, done: make(chan struct{}), joined: make(chan struct{})}
	*live = *tracer.Hooks
	go func() {
		defer close(session.joined)
		select {
		case <-ctx.Done():
			tracer.Stop(ctx.Err())
			evm.Cancel()
		case <-session.done:
		}
	}()
	if live.OnTxStart != nil {
		live.OnTxStart(evm.GetVMContext(), tx, from)
	}
	return session, nil
}

func (s *simulationTraceSession) end(receipt *types.Receipt, err error) {
	if s != nil && s.live.OnTxEnd != nil {
		s.live.OnTxEnd(receipt, err)
	}
}

func (s *simulationTraceSession) close() {
	if s == nil {
		return
	}
	close(s.done)
	<-s.joined
	*s.live = *s.legacy
}
