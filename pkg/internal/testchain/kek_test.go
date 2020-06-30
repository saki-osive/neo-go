package testchain

import (
	"fmt"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
)

func TestKek(t *testing.T) {
	for _, s := range privNetKeys {
		fmt.Println(s)
		priv, _ := keys.NewPrivateKeyFromWIF(s)
		fmt.Println(priv.Address())
	}
}
