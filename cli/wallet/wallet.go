package wallet

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/nspcc-dev/neo-go/cli/flags"
	"github.com/nspcc-dev/neo-go/cli/input"
	"github.com/nspcc-dev/neo-go/cli/options"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
	"github.com/urfave/cli"
)

var (
	errNoPath         = errors.New("target path where the wallet should be stored is mandatory and should be passed using (--wallet, -w) flags")
	errPhraseMismatch = errors.New("the entered pass-phrases do not match. Maybe you have misspelled them")
)

var (
	walletPathFlag = cli.StringFlag{
		Name:  "wallet, w",
		Usage: "Target location of the wallet file.",
	}
	wifFlag = cli.StringFlag{
		Name:  "wif",
		Usage: "WIF to import",
	}
	decryptFlag = cli.BoolFlag{
		Name:  "decrypt, d",
		Usage: "Decrypt encrypted keys.",
	}
	outFlag = cli.StringFlag{
		Name:  "out",
		Usage: "file to put JSON transaction to",
	}
	inFlag = cli.StringFlag{
		Name:  "in",
		Usage: "file with JSON transaction",
	}
	fromAddrFlag = flags.AddressFlag{
		Name:  "from",
		Usage: "Address to send an asset from",
	}
	toAddrFlag = flags.AddressFlag{
		Name:  "to",
		Usage: "Address to send an asset to",
	}
	forceFlag = cli.BoolFlag{
		Name:  "force",
		Usage: "Do not ask for a confirmation",
	}
)

// NewCommands returns 'wallet' command.
func NewCommands() []cli.Command {
	claimFlags := []cli.Flag{
		walletPathFlag,
		flags.AddressFlag{
			Name:  "address, a",
			Usage: "Address to claim GAS for",
		},
	}
	claimFlags = append(claimFlags, options.RPC...)
	return []cli.Command{{
		Name:  "wallet",
		Usage: "create, open and manage a NEO wallet",
		Subcommands: []cli.Command{
			{
				Name:   "claim",
				Usage:  "claim GAS",
				Action: claimGas,
				Flags:  claimFlags,
			},
			{
				Name:   "init",
				Usage:  "create a new wallet",
				Action: createWallet,
				Flags: []cli.Flag{
					walletPathFlag,
					cli.BoolFlag{
						Name:  "account, a",
						Usage: "Create a new account",
					},
				},
			},
			{
				Name:   "convert",
				Usage:  "convert addresses from existing NEO2 NEP6-wallet to NEO3 format",
				Action: convertWallet,
				Flags: []cli.Flag{
					walletPathFlag,
					cli.StringFlag{
						Name:  "out, o",
						Usage: "where to write converted wallet",
					},
				},
			},
			{
				Name:   "create",
				Usage:  "add an account to the existing wallet",
				Action: addAccount,
				Flags: []cli.Flag{
					walletPathFlag,
				},
			},
			{
				Name:   "dump",
				Usage:  "check and dump an existing NEO wallet",
				Action: dumpWallet,
				Flags: []cli.Flag{
					walletPathFlag,
					decryptFlag,
				},
			},
			{
				Name:      "export",
				Usage:     "export keys for address",
				UsageText: "export --wallet <path> [--decrypt] [<address>]",
				Action:    exportKeys,
				Flags: []cli.Flag{
					walletPathFlag,
					decryptFlag,
				},
			},
			{
				Name:   "import",
				Usage:  "import WIF",
				Action: importWallet,
				Flags: []cli.Flag{
					walletPathFlag,
					wifFlag,
					cli.StringFlag{
						Name:  "name, n",
						Usage: "Optional account name",
					},
					cli.StringFlag{
						Name:  "contract",
						Usage: "Verification script for custom contracts",
					},
				},
			},
			{
				Name:  "import-multisig",
				Usage: "import multisig contract",
				UsageText: "import-multisig --wallet <path> --wif <wif> --min <n>" +
					" [<pubkey1> [<pubkey2> [...]]]",
				Action: importMultisig,
				Flags: []cli.Flag{
					walletPathFlag,
					wifFlag,
					cli.StringFlag{
						Name:  "name, n",
						Usage: "Optional account name",
					},
					cli.IntFlag{
						Name:  "min, m",
						Usage: "Minimal number of signatures",
					},
				},
			},
			{
				Name:      "import-deployed",
				Usage:     "import deployed contract",
				UsageText: "import-multisig --wallet <path> --wif <wif> --contract <hash>",
				Action:    importDeployed,
				Flags: append([]cli.Flag{
					walletPathFlag,
					wifFlag,
					cli.StringFlag{
						Name:  "contract, c",
						Usage: "Contract hash",
					},
				}, options.RPC...),
			},
			{
				Name:      "remove",
				Usage:     "remove an account from the wallet",
				UsageText: "remove --wallet <path> [--force] <addr>",
				Action:    removeAccount,
				Flags: []cli.Flag{
					walletPathFlag,
					forceFlag,
				},
			},
			{
				Name:        "multisig",
				Usage:       "work with multisig address",
				Subcommands: newMultisigCommands(),
			},
			{
				Name:        "nep5",
				Usage:       "work with NEP5 contracts",
				Subcommands: newNEP5Commands(),
			},
			{
				Name:        "candidate",
				Usage:       "work with candidates",
				Subcommands: newValidatorCommands(),
			},
		},
	}}
}

func claimGas(ctx *cli.Context) error {
	wall, err := openWallet(ctx.String("wallet"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	defer wall.Close()

	addrFlag := ctx.Generic("address").(*flags.Address)
	if !addrFlag.IsSet {
		return cli.NewExitError("address was not provided", 1)
	}
	scriptHash := addrFlag.Uint160()
	acc, err := getDecryptedAccount(ctx, wall, scriptHash)
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	gctx, cancel := options.GetTimeoutContext(ctx)
	defer cancel()

	c, err := options.GetRPCClient(gctx, ctx)
	if err != nil {
		return err
	}

	neoContractHash, err := c.GetNEOContractHash()
	if err != nil {
		return err
	}
	hash, err := c.TransferNEP5(acc, scriptHash, neoContractHash, 0, 0)
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	fmt.Fprintln(ctx.App.Writer, hash.StringLE())
	return nil
}

func convertWallet(ctx *cli.Context) error {
	wall, err := openWallet(ctx.String("wallet"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	defer wall.Close()

	newWallet, err := wallet.NewWallet(ctx.String("out"))
	if err != nil {
		return cli.NewExitError(err, -1)
	}
	defer newWallet.Close()

	for _, acc := range wall.Accounts {
		address.Prefix = address.NEO2Prefix

		pass, err := input.ReadPassword(ctx.App.Writer, fmt.Sprintf("Enter passphrase for account %s (label '%s') > ", acc.Address, acc.Label))
		if err != nil {
			return cli.NewExitError(err, -1)
		} else if err := acc.Decrypt(pass); err != nil {
			return cli.NewExitError("invalid passphrase", -1)
		}

		address.Prefix = address.NEO3Prefix
		newAcc, err := wallet.NewAccountFromWIF(acc.PrivateKey().WIF())
		if err != nil {
			return cli.NewExitError(fmt.Errorf("can't convert account: %w", err), -1)
		}
		newAcc.Address = address.Uint160ToString(acc.Contract.ScriptHash())
		newAcc.Contract = acc.Contract
		newAcc.Default = acc.Default
		newAcc.Label = acc.Label
		newAcc.Locked = acc.Locked
		if err := newAcc.Encrypt(pass); err != nil {
			return cli.NewExitError(fmt.Errorf("can't encrypt converted account: %w", err), -1)
		}
		newWallet.AddAccount(newAcc)
	}
	if err := newWallet.Save(); err != nil {
		return cli.NewExitError(err, -1)
	}
	return nil
}

func addAccount(ctx *cli.Context) error {
	wall, err := openWallet(ctx.String("wallet"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	defer wall.Close()

	if err := createAccount(ctx, wall); err != nil {
		return cli.NewExitError(err, 1)
	}

	return nil
}

func exportKeys(ctx *cli.Context) error {
	wall, err := openWallet(ctx.String("wallet"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	var addr string

	decrypt := ctx.Bool("decrypt")
	if ctx.NArg() == 0 && decrypt {
		return cli.NewExitError(errors.New("address must be provided if '--decrypt' flag is used"), 1)
	} else if ctx.NArg() > 0 {
		// check address format just to catch possible typos
		addr = ctx.Args().First()
		_, err := address.StringToUint160(addr)
		if err != nil {
			return cli.NewExitError(fmt.Errorf("can't parse address: %w", err), 1)
		}
	}

	var wifs []string

loop:
	for _, a := range wall.Accounts {
		if addr != "" && a.Address != addr {
			continue
		}

		for i := range wifs {
			if a.EncryptedWIF == wifs[i] {
				continue loop
			}
		}

		wifs = append(wifs, a.EncryptedWIF)
	}

	for _, wif := range wifs {
		if decrypt {
			pass, err := input.ReadPassword(ctx.App.Writer, "Enter password > ")
			if err != nil {
				return cli.NewExitError(err, 1)
			}

			pk, err := keys.NEP2Decrypt(wif, pass)
			if err != nil {
				return cli.NewExitError(err, 1)
			}

			wif = pk.WIF()
		}

		fmt.Fprintln(ctx.App.Writer, wif)
	}

	return nil
}

func importMultisig(ctx *cli.Context) error {
	wall, err := openWallet(ctx.String("wallet"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	defer wall.Close()

	m := ctx.Int("min")
	if ctx.NArg() < m {
		return cli.NewExitError(errors.New("insufficient number of public keys"), 1)
	}

	args := []string(ctx.Args())
	pubs := make([]*keys.PublicKey, len(args))

	for i := range args {
		pubs[i], err = keys.NewPublicKeyFromString(args[i])
		if err != nil {
			return cli.NewExitError(fmt.Errorf("can't decode public key %d: %w", i, err), 1)
		}
	}

	acc, err := newAccountFromWIF(ctx.App.Writer, ctx.String("wif"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	if err := acc.ConvertMultisig(m, pubs); err != nil {
		return cli.NewExitError(err, 1)
	}

	if err := addAccountAndSave(wall, acc); err != nil {
		return cli.NewExitError(err, 1)
	}

	return nil
}

func importDeployed(ctx *cli.Context) error {
	wall, err := openWallet(ctx.String("wallet"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	defer wall.Close()

	rawHash := strings.TrimPrefix(ctx.String("contract"), "0x")
	h, err := util.Uint160DecodeStringLE(rawHash)
	if err != nil {
		return cli.NewExitError(fmt.Errorf("invalid contract hash: %w", err), 1)
	}

	acc, err := newAccountFromWIF(ctx.App.Writer, ctx.String("wif"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	gctx, cancel := options.GetTimeoutContext(ctx)
	defer cancel()

	c, err := options.GetRPCClient(gctx, ctx)
	if err != nil {
		return err
	}

	cs, err := c.GetContractStateByHash(h)
	if err != nil {
		return cli.NewExitError(fmt.Errorf("can't fetch contract info: %w", err), 1)
	}
	md := cs.Manifest.ABI.GetMethod(manifest.MethodVerify)
	if md == nil {
		return cli.NewExitError("contract has no `verify` method", 1)
	}
	acc.Address = address.Uint160ToString(cs.ScriptHash())
	acc.Contract.Script = cs.Script
	acc.Contract.Parameters = acc.Contract.Parameters[:0]
	for _, p := range md.Parameters {
		acc.Contract.Parameters = append(acc.Contract.Parameters, wallet.ContractParam{
			Name: p.Name,
			Type: p.Type,
		})
	}
	acc.Contract.Deployed = true

	if err := addAccountAndSave(wall, acc); err != nil {
		return cli.NewExitError(err, 1)
	}

	return nil
}

func importWallet(ctx *cli.Context) error {
	wall, err := openWallet(ctx.String("wallet"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	defer wall.Close()

	acc, err := newAccountFromWIF(ctx.App.Writer, ctx.String("wif"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	if ctrFlag := ctx.String("contract"); ctrFlag != "" {
		ctr, err := hex.DecodeString(ctrFlag)
		if err != nil {
			return cli.NewExitError("invalid contract", 1)
		}
		acc.Contract.Script = ctr
	}

	if acc.Label == "" {
		acc.Label = ctx.String("name")
	}
	if err := addAccountAndSave(wall, acc); err != nil {
		return cli.NewExitError(err, 1)
	}

	return nil
}

func removeAccount(ctx *cli.Context) error {
	wall, err := openWallet(ctx.String("wallet"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	defer wall.Close()

	addrArg := ctx.Args().First()
	addr, err := address.StringToUint160(addrArg)
	if err != nil {
		return cli.NewExitError("valid address must be provided", 1)
	}
	acc := wall.GetAccount(addr)
	if acc == nil {
		return cli.NewExitError("account wasn't found", 1)
	}

	if !ctx.Bool("force") {
		fmt.Fprintf(ctx.App.Writer, "Account %s will be removed. This action is irreversible.\n", addrArg)
		if ok := askForConsent(ctx.App.Writer); !ok {
			return nil
		}
	}

	if err := wall.RemoveAccount(acc.Address); err != nil {
		return cli.NewExitError(fmt.Errorf("error on remove: %w", err), 1)
	} else if err := wall.Save(); err != nil {
		return cli.NewExitError(fmt.Errorf("error while saving wallet: %w", err), 1)
	}
	return nil
}

func askForConsent(w io.Writer) bool {
	response, err := input.ReadLine(w, "Are you sure? [y/N]: ")
	if err == nil {
		response = strings.ToLower(strings.TrimSpace(response))
		if response == "y" || response == "yes" {
			return true
		}
	}
	fmt.Fprintln(w, "Cancelled.")
	return false
}

func dumpWallet(ctx *cli.Context) error {
	wall, err := openWallet(ctx.String("wallet"))
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	if ctx.Bool("decrypt") {
		pass, err := input.ReadPassword(ctx.App.Writer, "Enter wallet password > ")
		if err != nil {
			return cli.NewExitError(err, 1)
		}
		for i := range wall.Accounts {
			// Just testing the decryption here.
			err := wall.Accounts[i].Decrypt(pass)
			if err != nil {
				return cli.NewExitError(err, 1)
			}
		}
	}
	fmtPrintWallet(ctx.App.Writer, wall)
	return nil
}

func createWallet(ctx *cli.Context) error {
	path := ctx.String("wallet")
	if len(path) == 0 {
		return cli.NewExitError(errNoPath, 1)
	}
	wall, err := wallet.NewWallet(path)
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	if err := wall.Save(); err != nil {
		return cli.NewExitError(err, 1)
	}

	if ctx.Bool("account") {
		if err := createAccount(ctx, wall); err != nil {
			return cli.NewExitError(err, 1)
		}
	}

	fmtPrintWallet(ctx.App.Writer, wall)
	fmt.Fprintf(ctx.App.Writer, "wallet successfully created, file location is %s\n", wall.Path())
	return nil
}

func readAccountInfo(w io.Writer) (string, string, error) {
	rawName, _ := input.ReadLine(w, "Enter the name of the account > ")
	phrase, err := input.ReadPassword(w, "Enter passphrase > ")
	if err != nil {
		return "", "", err
	}
	phraseCheck, err := input.ReadPassword(w, "Confirm passphrase > ")
	if err != nil {
		return "", "", err
	}

	if phrase != phraseCheck {
		return "", "", errPhraseMismatch
	}

	name := strings.TrimRight(string(rawName), "\n")
	return name, phrase, nil
}

func createAccount(ctx *cli.Context, wall *wallet.Wallet) error {
	name, phrase, err := readAccountInfo(ctx.App.Writer)
	if err != nil {
		return err
	}
	return wall.CreateAccount(name, phrase)
}

func openWallet(path string) (*wallet.Wallet, error) {
	if len(path) == 0 {
		return nil, errNoPath
	}
	return wallet.NewWalletFromFile(path)
}

func newAccountFromWIF(w io.Writer, wif string) (*wallet.Account, error) {
	// note: NEP2 strings always have length of 58 even though
	// base58 strings can have different lengths even if slice lengths are equal
	if len(wif) == 58 {
		pass, err := input.ReadPassword(w, "Enter password > ")
		if err != nil {
			return nil, err
		}

		return wallet.NewAccountFromEncryptedWIF(wif, pass)
	}

	acc, err := wallet.NewAccountFromWIF(wif)
	if err != nil {
		return nil, err
	}

	fmt.Fprintln(w, "Provided WIF was unencrypted. Wallet can contain only encrypted keys.")
	name, pass, err := readAccountInfo(w)
	if err != nil {
		return nil, err
	}

	acc.Label = name
	if err := acc.Encrypt(pass); err != nil {
		return nil, err
	}

	return acc, nil
}

func addAccountAndSave(w *wallet.Wallet, acc *wallet.Account) error {
	for i := range w.Accounts {
		if w.Accounts[i].Address == acc.Address {
			return fmt.Errorf("address '%s' is already in wallet", acc.Address)
		}
	}

	w.AddAccount(acc)
	return w.Save()
}

func fmtPrintWallet(w io.Writer, wall *wallet.Wallet) {
	b, _ := wall.JSON()
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, string(b))
	fmt.Fprintln(w, "")
}
