package wallet

import (
	"fmt"

	"github.com/nspcc-dev/neo-go/cli/flags"
	"github.com/nspcc-dev/neo-go/cli/input"
	"github.com/nspcc-dev/neo-go/cli/options"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/emit"
	"github.com/nspcc-dev/neo-go/pkg/vm/opcode"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
	"github.com/urfave/cli"
)

func newValidatorCommands() []cli.Command {
	return []cli.Command{
		{
			Name:      "register",
			Usage:     "register as a new candidate",
			UsageText: "register -w <path> -r <rpc> -a <addr>",
			Action:    handleRegister,
			Flags: append([]cli.Flag{
				walletPathFlag,
				gasFlag,
				flags.AddressFlag{
					Name:  "address, a",
					Usage: "Address to register",
				},
			}, options.RPC...),
		},
		{
			Name:      "unregister",
			Usage:     "unregister self as a candidate",
			UsageText: "unregister -w <path> -r <rpc> -a <addr>",
			Action:    handleUnregister,
			Flags: append([]cli.Flag{
				walletPathFlag,
				gasFlag,
				flags.AddressFlag{
					Name:  "address, a",
					Usage: "Address to unregister",
				},
			}, options.RPC...),
		},
		{
			Name:      "vote",
			Usage:     "vote for a validator",
			UsageText: "vote -w <path> -r <rpc> [-s <timeout>] [-g gas] -a <addr> -c <public key>",
			Action:    handleVote,
			Flags: append([]cli.Flag{
				walletPathFlag,
				gasFlag,
				flags.AddressFlag{
					Name:  "address, a",
					Usage: "Address to vote from",
				},
				cli.StringFlag{
					Name:  "candidate, c",
					Usage: "Public key of candidate to vote for",
				},
			}, options.RPC...),
		},
	}
}

func handleRegister(ctx *cli.Context) error {
	return handleCandidate(ctx, "registerCandidate")
}

func handleUnregister(ctx *cli.Context) error {
	return handleCandidate(ctx, "unregisterCandidate")
}

func handleCandidate(ctx *cli.Context, method string) error {
	wall, err := openWallet(ctx.String("wallet"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	defer wall.Close()

	addrFlag := ctx.Generic("address").(*flags.Address)
	addr := addrFlag.Uint160()
	acc, err := getDecryptedAccount(ctx, wall, addr)
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	gctx, cancel := options.GetTimeoutContext(ctx)
	defer cancel()

	c, err := options.GetRPCClient(gctx, ctx)
	if err != nil {
		return err
	}

	gas := flags.Fixed8FromContext(ctx, "gas")
	neoContractHash, err := c.GetNEOContractHash()
	if err != nil {
		return err
	}
	w := io.NewBufBinWriter()
	emit.AppCallWithOperationAndArgs(w.BinWriter, neoContractHash, method, acc.PrivateKey().PublicKey().Bytes())
	emit.Opcode(w.BinWriter, opcode.ASSERT)
	tx, err := c.CreateTxFromScript(w.Bytes(), acc, -1, int64(gas), transaction.Signer{
		Account: acc.Contract.ScriptHash(),
		Scopes:  transaction.CalledByEntry,
	})
	if err != nil {
		return cli.NewExitError(err, 1)
	} else if err = acc.SignTx(tx); err != nil {
		return cli.NewExitError(fmt.Errorf("can't sign tx: %v", err), 1)
	}

	res, err := c.SendRawTransaction(tx)
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	fmt.Fprintln(ctx.App.Writer, res.StringLE())
	return nil
}

func handleVote(ctx *cli.Context) error {
	wall, err := openWallet(ctx.String("wallet"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	defer wall.Close()

	addrFlag := ctx.Generic("address").(*flags.Address)
	addr := addrFlag.Uint160()
	acc, err := getDecryptedAccount(ctx, wall, addr)
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	var pub *keys.PublicKey
	pubStr := ctx.String("candidate")
	if pubStr != "" {
		pub, err = keys.NewPublicKeyFromString(pubStr)
		if err != nil {
			return cli.NewExitError(fmt.Errorf("invalid public key: '%s'", pubStr), 1)
		}
	}

	gctx, cancel := options.GetTimeoutContext(ctx)
	defer cancel()

	c, err := options.GetRPCClient(gctx, ctx)
	if err != nil {
		return err
	}

	var pubArg interface{}
	if pub != nil {
		pubArg = pub.Bytes()
	}

	gas := flags.Fixed8FromContext(ctx, "gas")
	neoContractHash, err := c.GetNEOContractHash()
	if err != nil {
		return err
	}
	w := io.NewBufBinWriter()
	emit.AppCallWithOperationAndArgs(w.BinWriter, neoContractHash, "vote", addr.BytesBE(), pubArg)
	emit.Opcode(w.BinWriter, opcode.ASSERT)

	tx, err := c.CreateTxFromScript(w.Bytes(), acc, -1, int64(gas), transaction.Signer{
		Account: acc.Contract.ScriptHash(),
		Scopes:  transaction.CalledByEntry,
	})
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	if err = acc.SignTx(tx); err != nil {
		return cli.NewExitError(fmt.Errorf("can't sign tx: %v", err), 1)
	}

	res, err := c.SendRawTransaction(tx)
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	fmt.Fprintln(ctx.App.Writer, res.StringLE())
	return nil
}

func getDecryptedAccount(ctx *cli.Context, wall *wallet.Wallet, addr util.Uint160) (*wallet.Account, error) {
	acc := wall.GetAccount(addr)
	if acc == nil {
		return nil, fmt.Errorf("can't find account for the address: %s", address.Uint160ToString(addr))
	}

	if pass, err := input.ReadPassword(ctx.App.Writer, "Password > "); err != nil {
		fmt.Println("ERROR", pass, err)
		return nil, err
	} else if err := acc.Decrypt(pass); err != nil {
		return nil, err
	}
	return acc, nil
}
