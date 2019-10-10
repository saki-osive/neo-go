package core

import (
	"github.com/CityOfZion/neo-go/pkg/core/storage"
	"github.com/CityOfZion/neo-go/pkg/crypto/hash"
	"github.com/CityOfZion/neo-go/pkg/io"
	"github.com/CityOfZion/neo-go/pkg/smartcontract"
	"github.com/CityOfZion/neo-go/pkg/util"
)

// Contracts is a mapping between scripthash and ContractState.
type Contracts map[util.Uint160]*ContractState

// ContractState holds information about a smart contract in the NEO blockchain.
type ContractState struct {
	Script      []byte
	ParamList   []smartcontract.ParamType
	ReturnType  smartcontract.ParamType
	Properties  smartcontract.PropertyState
	Name        string
	CodeVersion string
	Author      string
	Email       string
	Description string

	scriptHash util.Uint160
}

// commit flushes all contracts to the given storage.Batch.
func (a Contracts) commit(b storage.Batch) error {
	buf := io.NewBufBinWriter()
	for hash, contract := range a {
		contract.EncodeBinary(buf.BinWriter)
		if buf.Err != nil {
			return buf.Err
		}
		key := storage.AppendPrefix(storage.STContract, hash.Bytes())
		b.Put(key, buf.Bytes())
		buf.Reset()
	}
	return nil
}

// DecodeBinary implements Serializable interface.
func (a *ContractState) DecodeBinary(br *io.BinReader) {
	a.Script = br.ReadBytes()
	paramBytes := br.ReadBytes()
	a.ParamList = make([]smartcontract.ParamType, len(paramBytes))
	for k := range paramBytes {
		a.ParamList[k] = smartcontract.ParamType(paramBytes[k])
	}
	br.ReadLE(&a.ReturnType)
	br.ReadLE(&a.Properties)
	a.Name = br.ReadString()
	a.CodeVersion = br.ReadString()
	a.Author = br.ReadString()
	a.Email = br.ReadString()
	a.Description = br.ReadString()
	a.createHash()
}

// EncodeBinary implements Serializable interface.
func (a *ContractState) EncodeBinary(bw *io.BinWriter) {
	bw.WriteBytes(a.Script)
	bw.WriteVarUint(uint64(len(a.ParamList)))
	for k := range a.ParamList {
		bw.WriteLE(a.ParamList[k])
	}
	bw.WriteLE(a.ReturnType)
	bw.WriteLE(a.Properties)
	bw.WriteString(a.Name)
	bw.WriteString(a.CodeVersion)
	bw.WriteString(a.Author)
	bw.WriteString(a.Email)
	bw.WriteString(a.Description)
}

// ScriptHash returns a contract script hash.
func (a *ContractState) ScriptHash() util.Uint160 {
	if a.scriptHash.Equals(util.Uint160{}) {
		a.createHash()
	}
	return a.scriptHash
}

// createHash creates contract script hash.
func (a *ContractState) createHash() {
	a.scriptHash = hash.Hash160(a.Script)
}

// HasStorage checks whether the contract has storage property set.
func (cs *ContractState) HasStorage() bool {
	return (cs.Properties & smartcontract.HasStorage) != 0
}

// HasDynamicInvoke checks whether the contract has dynamic invoke property set.
func (cs *ContractState) HasDynamicInvoke() bool {
	return (cs.Properties & smartcontract.HasDynamicInvoke) != 0
}

// IsPayable checks whether the contract has payable property set.
func (cs *ContractState) IsPayable() bool {
	return (cs.Properties & smartcontract.IsPayable) != 0
}
