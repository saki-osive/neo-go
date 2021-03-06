package state

import (
	"math/big"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/internal/random"
	"github.com/nspcc-dev/neo-go/pkg/internal/testserdes"
	"github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/stretchr/testify/require"
)

func TestOracleRequest_EncodeBinary(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		r := &OracleRequest{
			OriginalTxID:     random.Uint256(),
			GasForResponse:   12345,
			URL:              "https://get.value",
			CallbackContract: random.Uint160(),
			CallbackMethod:   "method",
			UserData:         []byte{1, 2, 3},
		}
		testserdes.EncodeDecodeBinary(t, r, new(OracleRequest))

		t.Run("WithFilter", func(t *testing.T) {
			s := "filter"
			r.Filter = &s
			testserdes.EncodeDecodeBinary(t, r, new(OracleRequest))
		})
	})
	t.Run("Invalid", func(t *testing.T) {
		w := io.NewBufBinWriter()
		t.Run("NotArray", func(t *testing.T) {
			w.Reset()
			it := stackitem.NewByteArray([]byte{})
			stackitem.EncodeBinaryStackItem(it, w.BinWriter)
			require.Error(t, testserdes.DecodeBinary(w.Bytes(), new(OracleRequest)))
		})
		t.Run("NotStackItem", func(t *testing.T) {
			w.Reset()
			require.Error(t, testserdes.DecodeBinary([]byte{0x77}, new(OracleRequest)))
		})

		items := []stackitem.Item{
			stackitem.NewByteArray(random.Uint256().BytesBE()),
			stackitem.NewBigInteger(big.NewInt(123)),
			stackitem.Make("url"),
			stackitem.Null{},
			stackitem.NewByteArray(random.Uint160().BytesBE()),
			stackitem.Make("method"),
			stackitem.NewByteArray([]byte{1, 2, 3}),
		}
		arrItem := stackitem.NewArray(items)
		runInvalid := func(i int, elem stackitem.Item) func(t *testing.T) {
			return func(t *testing.T) {
				w.Reset()
				before := items[i]
				items[i] = elem
				stackitem.EncodeBinaryStackItem(arrItem, w.BinWriter)
				items[i] = before
				require.Error(t, testserdes.DecodeBinary(w.Bytes(), new(OracleRequest)))
			}
		}
		t.Run("TxID", func(t *testing.T) {
			t.Run("Type", runInvalid(0, stackitem.NewMap()))
			t.Run("Length", runInvalid(0, stackitem.NewByteArray([]byte{0, 1, 2})))
		})
		t.Run("Gas", runInvalid(1, stackitem.NewMap()))
		t.Run("URL", runInvalid(2, stackitem.NewMap()))
		t.Run("Filter", runInvalid(3, stackitem.NewMap()))
		t.Run("Contract", func(t *testing.T) {
			t.Run("Type", runInvalid(4, stackitem.NewMap()))
			t.Run("Length", runInvalid(4, stackitem.NewByteArray([]byte{0, 1, 2})))
		})
		t.Run("Method", runInvalid(5, stackitem.NewMap()))
		t.Run("UserData", runInvalid(6, stackitem.NewMap()))
	})

}
