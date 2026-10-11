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

package tests

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestTransactionForkCoverage(t *testing.T) {
	for _, fork := range []string{"EIP150", "EIP158", "ConstantinopleFix", "SpuriousDragon", "TangerineWhistle"} {
		t.Run(fork, func(t *testing.T) {
			t.Run("reject_invalid_with_expected_success", func(t *testing.T) {
				test := TransactionTest{
					Txbytes: hexutil.Bytes{0xff},
					Result: map[string]*ttFork{
						fork: {Hash: new(common.UnprefixedHash), Sender: new(common.UnprefixedAddress)},
					},
				}
				err := test.Run()
				want := "unexpected error fork " + fork + ":"
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("Run() error = %v, want %s", err, want)
				}
			})
			t.Run("accept_invalid_with_expected_error", func(t *testing.T) {
				exception := "invalid transaction"
				test := TransactionTest{
					Txbytes: hexutil.Bytes{0xff},
					Result:  map[string]*ttFork{fork: {Exception: &exception}},
				}
				if err := test.Run(); err != nil {
					t.Fatalf("Run() error = %v, want nil for expected invalid transaction", err)
				}
			})
		})
	}
}
