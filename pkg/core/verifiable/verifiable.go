package verifiable

import "github.com/nspcc-dev/neo-go/pkg/util"

type CheckWitnessHashes []util.Uint160

func (h CheckWitnessHashes) GetSignedPart() []byte {
	panic("Not implemented")
}
