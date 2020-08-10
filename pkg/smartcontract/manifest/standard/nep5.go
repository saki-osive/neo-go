package standard

import (
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
)

var nep5 = &manifest.Manifest{
	ABI: manifest.ABI{
		Methods: []manifest.Method{
			{
				Name: "balanceOf",
				Parameters: []manifest.Parameter{
					{Type: smartcontract.ByteArrayType},
				},
				ReturnType: smartcontract.IntegerType,
			},
			{
				Name:       "decimals",
				ReturnType: smartcontract.IntegerType,
			},
			{
				Name:       "name",
				ReturnType: smartcontract.StringType,
			},
			{
				Name:       "symbol",
				ReturnType: smartcontract.StringType,
			},
			{
				Name:       "totalSupply",
				ReturnType: smartcontract.IntegerType,
			},
			{
				Name: "transfer",
				Parameters: []manifest.Parameter{
					{Type: smartcontract.ByteArrayType},
					{Type: smartcontract.ByteArrayType},
					{Type: smartcontract.IntegerType},
				},
				ReturnType: smartcontract.BoolType,
			},
		},
		Events: []manifest.Event{
			{
				Name: "transfer",
				Parameters: []manifest.Parameter{
					{Type: smartcontract.ByteArrayType},
					{Type: smartcontract.ByteArrayType},
					{Type: smartcontract.IntegerType},
				},
			},
		},
	},
	Features: smartcontract.HasStorage,
}

// IsNEP5 checks if m is NEP-5 compliant.
func IsNEP5(m *manifest.Manifest) error {
	return Comply(m, nep5)
}
