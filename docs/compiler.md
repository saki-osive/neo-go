# NEO-GO smart contract compiler

The neo-go compiler compiles Go programs to bytecode that the NEO virtual machine can understand.

## Language compatibility

The compiler is mostly compatible with regular Go language specification, but
there are some important deviations that you need to be aware of that make it
a dialect of Go rather than a complete port of the language:
 * `new()` is not supported, most of the time you can substitute structs with composite literals
 * `make()` is supported for maps and slices with elements of basic types
 * `copy()` is supported only for byte slices, because of underlying `MEMCPY` opcode
 * pointers are supported only for struct literals, one can't take an address
   of an arbitrary variable
 * there is no real distinction between different integer types, all of them
   work as big.Int in Go with a limit of 256 bit in width, so you can use
   `int` for just about anything. This is the way integers work in Neo VM and
   adding proper Go types emulation is considered to be too costly.
 * goroutines, channels and garbage collection are not supported and will
   never be because emulating that aspects of Go runtime on top of Neo VM is
   close to impossible
 * `defer` and `recover` are supported except for cases where panic occurs in
   `return` statement, because this complicates implementation and imposes runtime
    overhead for all contracts. This can easily be mitigated by first storing values
    in variables and returning the result.
 * lambdas are supported, but closures are not.
 * maps are supported, but valid map keys are booleans, integers and strings with length <= 64

## VM API (interop layer)
Compiler translates interop function calls into NEO VM syscalls or (for custom
functions) into NEO VM instructions. [Refer to
pkg.go.dev](https://pkg.go.dev/github.com/nspcc-dev/neo-go/pkg/interop)
for full API documentation. In general it provides the same level of
functionality as Neo .net Framework library.

Compiler provides some helpful builtins in `util` and `convert` packages.
Refer to them for detailed documentation. 

`_deploy()` function has a special meaning and is executed when contract is deployed.
It should return no value and accept single bool argument which will be true on contract update.
`_deploy()` functions are called for every imported package in the same order as `init()`. 

## Quick start

### Compiling

```
./bin/neo-go contract compile -i mycontract.go
```

By default the filename will be the name of your .go file with the .nef extension, the file will be located in the same directory where your Go contract is. If you want another location for your compiled contract:

```
./bin/neo-go contract compile -i mycontract.go --out /Users/foo/bar/contract.nef
```

If you contract is split across multiple files, you must provide a path
to the directory where package files are contained instead of a single Go file:
```
./bin/neo-go contract compile -i ./path/to/contract
```

### Debugging
You can dump the opcodes generated by the compiler with the following command:

```
./bin/neo-go contract inspect -i mycontract.go -c
```

This will result in something like this:

```
INDEX    OPCODE      PARAMETER            
0        INITSLOT    0500 ("\x05\x00")    <<
3        PUSH0                            
4        REVERSEN                         
5        SYSCALL     "\x9a\x1f\x19J"      
10       NOP                              
11       STLOC0                           
12       LDLOC0                           
13       PUSH1                            
14       REVERSEN                         
15       PUSH1                            
16       PACK                             
17       SYSCALL     "\x05\a\x92\x16"     
22       NOP                              
23       PUSH0                            
24       REVERSEN                         
25       SYSCALL     "E\x99Z\\"           
30       NOP                              
31       STLOC1                           
32       LDLOC1                           
33       PUSH1                            
34       REVERSEN                         
35       PUSH1                            
36       PACK                             
37       SYSCALL     "\x05\a\x92\x16"     
42       NOP                              
43       PUSH0                            
44       REVERSEN                         
45       SYSCALL     "\x87\xc3\xd2d"      
50       NOP                              
51       STLOC2                           
52       LDLOC2                           
53       PUSH1                            
54       REVERSEN                         
55       PUSH1                            
56       PACK                             
57       SYSCALL     "\x05\a\x92\x16"     
62       NOP                              
63       PUSH0                            
64       REVERSEN                         
65       SYSCALL     "\x1dY\xe1\x19"      
70       NOP                              
71       STLOC3                           
72       LDLOC3                           
73       PUSH1                            
74       REVERSEN                         
75       PUSH1                            
76       PACK                             
77       SYSCALL     "\x05\a\x92\x16"     
82       NOP                              
83       PUSH1                            
84       RET                              
```

#### Neo Smart Contract Debugger support

It's possible to debug contracts written in Go using standard [Neo Smart
Contract Debugger](https://github.com/neo-project/neo-debugger/) which is a
part of [Neo Blockchain
Toolkit](https://github.com/neo-project/neo-blockchain-toolkit/). To do that
you need to generate debug information using `--debug` option, like this:

```
$ ./bin/neo-go contract compile -i contract.go -o contract.nef --debug contract.debug.json
```

This file can then be used by debugger and set up to work just like for any
other supported language.

### Deploying

Deploying a contract to blockchain with neo-go requires a configuration file
with contract's metadata in YAML format, like the following:

```
project:
  author: Jack Smith
  email: jack@example.com
  version: 1.0
  name: 'Smart contract'
  description: 'Even smarter than Jack himself'
  hasstorage: true
  hasdynamicinvocation: false
  ispayable: false
  returntype: ByteArray
  parameters: ['String', 'Array']
```

It's passed to the `deploy` command via `-c` option:

```
$ ./bin/neo-go contract deploy -i contract.nef -c contract.yml -r http://localhost:20331 -w wallet.json -g 0.001
```

Deployment works via an RPC server, an address of which is passed via `-r`
option and should be signed using a wallet from `-w` option. More details can
be found in `deploy` command help.

#### Neo Express support

It's possible to deploy contracts written in Go using [Neo
Express](https://github.com/neo-project/neo-express) which is a part of [Neo
Blockchain
Toolkit](https://github.com/neo-project/neo-blockchain-toolkit/). To do that
you need to generate a different metadata file using YAML written for
deployment with neo-go. It's done in the same step with compilation via
`--config` input parameter and `--abi` output parameter, combined with debug
support the command line will look like this:

```
$ ./bin/neo-go contract compile -i contract.go --config contract.yml -o contract.nef --debug contract.debug.json --abi contract.abi.json 
```

This file can then be used by toolkit to deploy contract the same way
contracts in other languagues are deployed.


### Invoking
You can import your contract into the standalone VM and run it there (see [VM
documentation](vm.md) for more info), but that only works for simple contracts
that don't use blockchain a lot. For more real contracts you need to deploy
them first and then do test invocations and regular invocations with `contract
testinvokefunction` and `contract invokefunction` commands (or their variants,
see `contract` command help for more details. They all work via RPC, so it's a
mandatory parameter.

Example call (contract `f84d6a337fbc3d3a201d41da99e86b479e7a2554` with method
`balanceOf` and method's parameter `AK2nJJpJr6o664CWJKi1QRXjqeic2zRp8y` using
given RPC server and wallet and paying 0.00001 GAS for this transaction):

```
$ ./bin/neo-go contract invokefunction -r http://localhost:20331 -w my_wallet.json -g 0.00001 f84d6a337fbc3d3a201d41da99e86b479e7a2554 balanceOf AK2nJJpJr6o664CWJKi1QRXjqeic2zRp8y
```

## Smart contract examples

Some examples are provided in the [examples directory](../examples).

### Check if the invoker of the contract is the owning address

```Golang
package mycontract

import (
    "github.com/nspcc-dev/neo-go/pkg/interop/runtime"
    "github.com/nspcc-dev/neo-go/pkg/interop/util"
)

var owner = util.FromAddress("AJX1jGfj3qPBbpAKjY527nPbnrnvSx9nCg") 

func Main() bool {
    isOwner := runtime.CheckWitness(owner)

    if isOwner {
        runtime.Log("invoker is the owner")
        return true
    }

    return false
}
```

### Simple token

```Golang
package mytoken

import (
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

var owner = util.FromAddress("AJX1jGfj3qPBbpAKjY527nPbnrnvSx9nCg") 

type Token struct {
	Name        string
	Symbol      string
	TotalSupply int
	Owner       []byte
}

func (t Token) AddToCirculation(amount int) bool {
	ctx := storage.Context()
	var inCirc int
	val := storage.Get(ctx, "in_circ")
	if val != nil {
		inCirc = val.(int)
	}
	inCirc += amount
	storage.Put(ctx, "in_circ", inCirc)
	return true
}

func newToken() Token {
	return Token{
		Name:        "your awesome NEO token",
		Symbol:      "YANT",
		TotalSupply: 1000,
		Owner:       owner,
	}
}

func Main(operation string, args []interface{}) bool {
	token := newToken()
	trigger := runtime.GetTrigger()

	if trigger == runtime.Verification {
		isOwner := runtime.CheckWitness(token.Owner)
		if isOwner {
			return true
		}
		return false
	}

	if trigger == runtime.Application {
		if operation == "mintTokens" {
			token.AddToCirculation(100)
		}
	}

	return true
}
```

## How to report compiler bugs 
1. Make a proper testcase (example testcases can be found in the tests folder)
2. Create an issue on Github 
3. Make a PR with a reference to the created issue, containing the testcase that proves the bug
4. Either you fix the bug yourself or wait for patch that solves the problem
