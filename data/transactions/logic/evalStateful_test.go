// Copyright (C) 2019-2023 Algorand, Inc.
// This file is part of go-algorand
//
// go-algorand is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// go-algorand is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with go-algorand.  If not, see <https://www.gnu.org/licenses/>.

package logic

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/test/partitiontest"
)

func makeApp(li uint64, lb uint64, gi uint64, gb uint64) basics.AppParams {
	return basics.AppParams{
		ApprovalProgram:   []byte{},
		ClearStateProgram: []byte{},
		GlobalState:       map[string]basics.TealValue{},
		StateSchemas: basics.StateSchemas{
			LocalStateSchema:  basics.StateSchema{NumUint: li, NumByteSlice: lb},
			GlobalStateSchema: basics.StateSchema{NumUint: gi, NumByteSlice: gb},
		},
		ExtraProgramPages: 0,
	}
}

func makeSampleEnv() (*EvalParams, *transactions.Transaction, *Ledger) {
	return makeSampleEnvWithVersion(LogicVersion)
}

func makeSampleEnvWithVersion(version uint64) (*EvalParams, *transactions.Transaction, *Ledger) {
	// We'd usually like an app in the group, so that the ep created is
	// "complete".  But to keep as many old tests working as possible, if
	// version < appsEnabledVersion, don't put an appl txn in it.
	firstTxn := makeSampleTxn()
	if version >= appsEnabledVersion {
		firstTxn.Txn.Type = protocol.ApplicationCallTx
	}
	ep := defaultEvalParamsWithVersion(version, makeSampleTxnGroup(firstTxn)...)
	ledger := NewLedger(nil)
	ep.SigLedger = ledger
	ep.Ledger = ledger
	return ep, &ep.TxnGroup[0].Txn, ledger
}

func makeOldAndNewEnv(version uint64) (*EvalParams, *EvalParams, *Ledger) {
	new, _, sharedLedger := makeSampleEnvWithVersion(version)
	old, _, _ := makeSampleEnvWithVersion(version - 1)
	old.Ledger = sharedLedger
	return old, new, sharedLedger
}

func (r *resources) String() string {
	sb := strings.Builder{}
	if len(r.createdAsas) > 0 {
		fmt.Fprintf(&sb, "createdAsas: %v\n", r.createdAsas)
	}
	if len(r.createdApps) > 0 {
		fmt.Fprintf(&sb, "createdApps: %v\n", r.createdApps)
	}

	if len(r.sharedAccounts) > 0 {
		fmt.Fprintf(&sb, "sharedAccts:\n")
		for addr := range r.sharedAccounts {
			fmt.Fprintf(&sb, " %s\n", addr)
		}
	}
	if len(r.sharedAsas) > 0 {
		fmt.Fprintf(&sb, "sharedAsas:\n")
		for id := range r.sharedAsas {
			fmt.Fprintf(&sb, " %d\n", id)
		}
	}
	if len(r.sharedApps) > 0 {
		fmt.Fprintf(&sb, "sharedApps:\n")
		for id := range r.sharedApps {
			fmt.Fprintf(&sb, " %d\n", id)
		}
	}

	if len(r.sharedHoldings) > 0 {
		fmt.Fprintf(&sb, "sharedHoldings:\n")
		for hl := range r.sharedHoldings {
			fmt.Fprintf(&sb, " %s x %d\n", hl.Address, hl.Asset)
		}
	}
	if len(r.sharedLocals) > 0 {
		fmt.Fprintf(&sb, "sharedLocals:\n")
		for hl := range r.sharedLocals {
			fmt.Fprintf(&sb, " %s x %d\n", hl.Address, hl.App)
		}
	}

	return sb.String()
}

func TestEvalModes(t *testing.T) {
	partitiontest.PartitionTest(t)

	t.Parallel()
	// ed25519verify* and err are tested separately below

	// check modeAny (v1 + txna/gtxna) are available in RunModeSignature
	// check all opcodes available in runModeApplication
	opcodesRunModeAny := `intcblock 0 1 1 1 1 5 100
	bytecblock "ALGO" 0x1337 0x2001 0xdeadbeef 0x70077007
bytec 0
sha256
keccak256
sha512_256
sha3_256
len
intc_0
+
intc_1
-
intc_2
/
intc_3
*
intc 4
<
intc_1
>
intc_1
<=
intc_1
>=
intc_1
&&
intc_1
||
bytec_1
bytec_2
!=
bytec_3
bytec 4
==
!
itob
btoi
%	// use values left after bytes comparison
|
intc_1
&
txn Fee
^
global MinTxnFee
~
gtxn 0 LastValid
mulw
pop
store 0
load 0
bnz label
label:
dup
pop
txna Accounts 0
gtxna 0 ApplicationArgs 0
==
`
	opcodesRunModeSignature := `arg_0
arg_1
!=
arg_2
arg_3
!=
&&
txn Sender
arg 4
!=
&&
!=
&&
`

	opcodesRunModeApplication := `txn Sender
balance
&&
txn Sender
min_balance
&&
txn Sender
intc 6  // 100
app_opted_in
&&
txn Sender
bytec_0 // ALGO
intc_1
app_local_put
bytec_0
intc_1
app_global_put
txn Sender
intc 6
bytec_0
app_local_get_ex
pop
&&
int 0
bytec_0
app_global_get_ex
pop
&&
txn Sender
bytec_0
app_local_del
bytec_0
app_global_del
txn Sender
intc 5 // 5
asset_holding_get AssetBalance
pop
&&
intc 5 // 5
asset_params_get AssetTotal
pop
&&
!=
bytec_0
log
`
	tests := map[RunMode]string{
		ModeSig: opcodesRunModeAny + opcodesRunModeSignature,
		ModeApp: opcodesRunModeAny + opcodesRunModeApplication,
	}

	for mode, test := range tests {
		mode, test := mode, test
		t.Run(fmt.Sprintf("opcodes_mode=%d", mode), func(t *testing.T) {
			t.Parallel()

			ep, tx, ledger := makeSampleEnv()
			ep.TxnGroup[0].Lsig.Args = [][]byte{
				tx.Sender[:],
				tx.Receiver[:],
				tx.CloseRemainderTo[:],
				tx.VotePK[:],
				tx.SelectionPK[:],
				tx.Note,
			}
			ep.TxnGroup[0].Txn.ApplicationID = 100
			ep.TxnGroup[0].Txn.ForeignAssets = []basics.AssetIndex{5} // needed since v4
			params := basics.AssetParams{
				Total:         1000,
				Decimals:      2,
				DefaultFrozen: false,
				UnitName:      "ALGO",
				AssetName:     "",
				URL:           string(protocol.PaymentTx),
				Manager:       tx.Sender,
				Reserve:       tx.Receiver,
				Freeze:        tx.Receiver,
				Clawback:      tx.Receiver,
			}
			algoValue := basics.TealValue{Type: basics.TealUintType, Uint: 0x77}
			ledger.NewAccount(tx.Sender, 1)
			ledger.NewApp(tx.Sender, 100, basics.AppParams{})
			ledger.NewLocals(tx.Sender, 100)
			ledger.NewLocal(tx.Sender, 100, "ALGO", algoValue)
			ledger.NewAsset(tx.Sender, 5, params)

			if mode == ModeSig {
				testLogic(t, test, AssemblerMaxVersion, ep)
			} else {
				testApp(t, test, ep)
			}
		})
	}

	// check err opcode work in both modes
	source := "err"
	testLogic(t, source, AssemblerMaxVersion, defaultEvalParams(), "err opcode executed")
	testApp(t, source, defaultEvalParams(), "err opcode executed")

	// check that ed25519verify and arg is not allowed in stateful mode between v2-v4
	disallowedV4 := []string{
		"byte 0x01\nbyte 0x01\nbyte 0x01\ned25519verify",
		"arg 0",
		"arg_0",
		"arg_1",
		"arg_2",
		"arg_3",
	}
	for _, source := range disallowedV4 {
		ops := testProg(t, source, 4)
		testAppBytes(t, ops.Program, defaultEvalParams(),
			"not allowed in current mode", "not allowed in current mode")
	}

	// check that arg is not allowed in stateful mode beyond v5
	disallowed := []string{
		"arg 0",
		"arg_0",
		"arg_1",
		"arg_2",
		"arg_3",
	}
	for _, source := range disallowed {
		ops := testProg(t, source, AssemblerMaxVersion)
		testAppBytes(t, ops.Program, defaultEvalParams(),
			"not allowed in current mode", "not allowed in current mode")
	}

	// check stateful opcodes are not allowed in stateless mode
	statefulOpcodeCalls := []string{
		"txn Sender; balance",
		"txn Sender; min_balance",
		"txn Sender; int 0; app_opted_in",
		"txn Sender; int 0; byte 0x01; app_local_get_ex",
		"byte 0x01; app_global_get",
		"int 0; byte 0x01; app_global_get_ex",
		"txn Sender; byte 0x01; byte 0x01; app_local_put",
		"byte 0x01; int 0; app_global_put",
		"txn Sender; byte 0x01; app_local_del",
		"byte 0x01; app_global_del",
		"txn Sender; int 0; asset_holding_get AssetFrozen",
		"int 0; int 0; asset_params_get AssetManager",
		"int 0; int 0; app_params_get AppApprovalProgram",
		"byte 0x01; log",
	}

	for _, source := range statefulOpcodeCalls {
		source := source
		testLogic(t, source, AssemblerMaxVersion, defaultEvalParams(),
			"not allowed in current mode", "not allowed in current mode")
	}

	require.Equal(t, RunMode(1), ModeSig)
	require.Equal(t, RunMode(2), ModeApp)
	require.True(t, modeAny == ModeSig|ModeApp)
	require.True(t, modeAny.Any())
}

func TestBalance(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	testLogicRange(t, 2, 0, func(t *testing.T, ep *EvalParams, tx *transactions.Transaction, ledger *Ledger) {
		ledger.NewAccount(tx.Receiver, 177)
		if ep.Proto.LogicSigVersion < sharedResourcesVersion {
			testApp(t, "int 2; balance; int 177; ==", ep, "invalid Account reference")
			testApp(t, `int 1; balance; int 177; ==`, ep)
		}

		source := `txn Accounts 1; balance; int 177; ==;`
		// won't assemble in old version teal
		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			testProg(t, source, ep.Proto.LogicSigVersion,
				Expect{1, "balance arg 0 wanted type uint64..."})
			return
		}

		// but legal after that
		testApp(t, source, ep)

		source = "txn Sender; balance; int 13; ==; assert; int 1"
		testApp(t, source, ep, "assert failed")

		ledger.NewAccount(tx.Sender, 13)
		testApp(t, source, ep)
	})
}

func testApps(t *testing.T, programs []string, txgroup []transactions.SignedTxn, version uint64, ledger *Ledger,
	expected ...Expect) *EvalParams {
	t.Helper()
	codes := make([][]byte, len(programs))
	for i, program := range programs {
		if program != "" {
			codes[i] = testProg(t, program, version).Program
		}
	}
	if txgroup == nil {
		for _, program := range programs {
			sample := makeSampleTxn()
			if program != "" {
				sample.Txn.Type = protocol.ApplicationCallTx
			}
			txgroup = append(txgroup, sample)
		}
	}
	ep := NewEvalParams(transactions.WrapSignedTxnsWithAD(txgroup), makeTestProtoV(version), &transactions.SpecialAddresses{})
	if ledger == nil {
		ledger = NewLedger(nil)
	}
	ledger.Reset()
	ep.Ledger = ledger
	ep.SigLedger = ledger
	testAppsBytes(t, codes, ep, expected...)
	return ep
}

func testAppsBytes(t *testing.T, programs [][]byte, ep *EvalParams, expected ...Expect) {
	t.Helper()
	require.LessOrEqual(t, len(programs), len(ep.TxnGroup))
	for i := range ep.TxnGroup {
		program := ep.TxnGroup[i].Txn.ApprovalProgram
		if len(programs) > i && programs[i] != nil {
			program = programs[i]
		}
		if program != nil {
			appID := ep.TxnGroup[i].Txn.ApplicationID
			if appID == 0 {
				appID = basics.AppIndex(888)
			}
			if len(expected) > 0 && expected[0].l == i {
				testAppFull(t, program, i, appID, ep, expected[0].s)
				break // Stop after first failure
			} else {
				testAppFull(t, program, i, appID, ep)
			}
		}
	}
}

func testApp(t *testing.T, program string, ep *EvalParams, problems ...string) transactions.EvalDelta {
	t.Helper()
	ops := testProg(t, program, ep.Proto.LogicSigVersion)
	return testAppBytes(t, ops.Program, ep, problems...)
}

func testAppBytes(t *testing.T, program []byte, ep *EvalParams, problems ...string) transactions.EvalDelta {
	t.Helper()
	ep.reset()
	aid := ep.TxnGroup[0].Txn.ApplicationID
	if aid == 0 {
		aid = basics.AppIndex(888)
	}
	return testAppFull(t, program, 0, aid, ep, problems...)
}

// testAppFull gives a lot of control to caller - in particular, notice that
// ep.reset() is in testAppBytes, not here. This means that ADs in the ep are
// not cleared, so repeated use of a single ep is probably not a good idea
// unless you are *intending* to see how ep is modified as you go.
func testAppFull(t *testing.T, program []byte, gi int, aid basics.AppIndex, ep *EvalParams, problems ...string) transactions.EvalDelta {
	t.Helper()

	var checkProblem string
	var evalProblem string
	switch len(problems) {
	case 2:
		checkProblem = problems[0]
		evalProblem = problems[1]
	case 1:
		evalProblem = problems[0]
	case 0:
		// no problems == expect success
	default:
		require.Fail(t, "Misused testApp: %d problems", len(problems))
	}

	sb := &strings.Builder{}
	ep.Trace = sb

	err := CheckContract(program, ep)
	if checkProblem == "" {
		require.NoError(t, err, sb.String())
	} else {
		require.Error(t, err, "Check\n%s\nExpected: %v", sb, checkProblem)
		require.Contains(t, err.Error(), checkProblem, sb.String())
	}

	// We continue on to check Eval() of things that failed Check() because it's
	// a nice confirmation that Check() is usually stricter than Eval(). This
	// may mean that the problems argument is often duplicated, but this seems
	// the best way to be concise about all sorts of tests.

	if ep.Ledger == nil {
		ep.Ledger = NewLedger(nil)
	}

	pass, err := EvalApp(program, gi, aid, ep)
	delta := ep.TxnGroup[gi].EvalDelta
	if evalProblem == "" {
		require.NoError(t, err, "Eval%s\nExpected: PASS", sb)
		require.True(t, pass, "Eval%s\nExpected: PASS", sb)
		return delta
	}

	// There is an evalProblem to check. REJECT is special and only means that
	// the app didn't accept.  Maybe it's an error, maybe it's just !pass.
	if evalProblem == "REJECT" {
		require.True(t, err != nil || !pass, "Eval%s\nExpected: REJECT", sb)
	} else {
		require.Error(t, err, "Eval\n%s\nExpected: %v", sb, evalProblem)
		require.Contains(t, err.Error(), evalProblem)
	}
	return delta
}

// testLogicRange allows for running tests against a range of avm
// versions. Generally `start` will be the version that introduced the feature,
// and `stop` will be 0 to indicate it should work right on up through the
// current version.  `stop` will be an actual version number if we're confirming
// that something STOPS working as of a particular version. Note that this does
// *not* use different consensus versions. It is tempting to make it find the
// lowest possible consensus version in the loop in order to support the `v` it
// it working on.  For super confidence, one might argue this should be a nested
// loop over all of the consensus versions that work with the `v`, from the
// first possible, to vFuture.
func testLogicRange(t *testing.T, start, stop int, test func(t *testing.T, ep *EvalParams, tx *transactions.Transaction, ledger *Ledger)) {
	t.Helper()
	if stop == 0 { // Treat 0 as current max
		stop = LogicVersion
	}

	for v := uint64(start); v <= uint64(stop); v++ {
		t.Run(fmt.Sprintf("v=%d", v), func(t *testing.T) {
			ep, tx, ledger := makeSampleEnvWithVersion(v)
			test(t, ep, tx, ledger)
		})
	}
}

func TestMinBalance(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	// since v3 is before directRefEnabledVersion, do a quick test on it separately
	ep, tx, ledger := makeSampleEnvWithVersion(3)
	ledger.NewAccount(tx.Sender, 100)

	testApp(t, "int 0; min_balance; int 1001; ==", ep)
	// Sender makes an asset, min balance goes up
	ledger.NewAsset(tx.Sender, 7, basics.AssetParams{Total: 1000})
	testApp(t, "int 0; min_balance; int 2002; ==", ep)

	// now test in more detail v4 and on
	testLogicRange(t, 4, 0, func(t *testing.T, ep *EvalParams, tx *transactions.Transaction, ledger *Ledger) {
		ledger.NewAccount(tx.Sender, 234)
		ledger.NewAccount(tx.Receiver, 123)

		testApp(t, "txn Sender; min_balance; int 1001; ==", ep)
		// Sender makes an asset, min balance goes up
		ledger.NewAsset(tx.Sender, 7, basics.AssetParams{Total: 1000})
		testApp(t, "txn Sender; min_balance; int 2002; ==", ep)
		schemas := makeApp(1, 2, 3, 4)
		ledger.NewApp(tx.Sender, 77, schemas)
		ledger.NewLocals(tx.Sender, 77)
		// create + optin + 10 schema base + 4 ints + 6 bytes (local
		// and global count b/c NewLocals opts the creator in)
		minb := 1002 + 1006 + 10*1003 + 4*1004 + 6*1005
		testApp(t, fmt.Sprintf("txn Sender; min_balance; int %d; ==", 2002+minb), ep)
		// request extra program pages, min balance increase
		withepp := makeApp(1, 2, 3, 4)
		withepp.ExtraProgramPages = 2
		ledger.NewApp(tx.Sender, 77, withepp)
		minb += 2 * 1002
		testApp(t, fmt.Sprintf("txn Sender; min_balance; int %d; ==", 2002+minb), ep)

		testApp(t, "txn Accounts 1; min_balance; int 1001; ==", ep)
		// Receiver opts in
		ledger.NewHolding(tx.Receiver, 7, 1, true)
		testApp(t, "txn Receiver; min_balance; int 2002; ==", ep) // 1 == Accounts[0]
	})
}

func TestAppCheckOptedIn(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	pre, now, ledger := makeOldAndNewEnv(directRefEnabledVersion)

	txn := pre.TxnGroup[0]
	ledger.NewAccount(txn.Txn.Receiver, 1)
	ledger.NewAccount(txn.Txn.Sender, 1)
	testApp(t, "int 2; int 100; app_opted_in; int 1; ==", now, "invalid Account reference")

	// Receiver is not opted in
	testApp(t, "int 1; int 100; app_opted_in; int 0; ==", now)
	testApp(t, "int 1; int 0; app_opted_in; int 0; ==", now)
	// These two give the same result, for different reasons
	testApp(t, "int 1; int 3; app_opted_in; int 0; ==", now) // refers to tx.ForeignApps[2], which is 111
	testApp(t, "int 1; int 3; app_opted_in; int 0; ==", pre) // not an indirect reference: actually app 3
	// 0 is a legal way to refer to the current app, even in pre (though not in spec)
	// but current app is 888 - not opted in
	testApp(t, "int 1; int 0; app_opted_in; int 0; ==", pre)

	// Sender is not opted in
	testApp(t, "int 0; int 100; app_opted_in; int 0; ==", now)

	// Receiver opted in
	ledger.NewLocals(txn.Txn.Receiver, 100)
	testApp(t, "int 1; int 100; app_opted_in; int 1; ==", now)
	testApp(t, "int 1; int 2; app_opted_in; int 1; ==", now) // tx.ForeignApps[1] == 100
	testApp(t, "int 1; int 2; app_opted_in; int 0; ==", pre) // in pre, int 2 is an actual app id
	testApp(t, "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui01\"; int 2; app_opted_in; int 1; ==", now)
	testProg(t, "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui01\"; int 2; app_opted_in; int 1; ==", directRefEnabledVersion-1,
		Expect{1, "app_opted_in arg 0 wanted type uint64..."})

	// Receiver opts into 888, the current app in testApp
	ledger.NewLocals(txn.Txn.Receiver, 888)
	// int 0 is current app (888) even in pre
	testApp(t, "int 1; int 0; app_opted_in; int 1; ==", pre)
	// Here it is "obviously" allowed, because indexes became legal
	testApp(t, "int 1; int 0; app_opted_in; int 1; ==", now)

	// Sender opted in
	ledger.NewLocals(txn.Txn.Sender, 100)
	testApp(t, "int 0; int 100; app_opted_in; int 1; ==", now)
}

func TestAppReadLocalState(t *testing.T) {
	partitiontest.PartitionTest(t)

	t.Parallel()

	text := `int 2  // account idx
int 100 // app id
txn ApplicationArgs 0
app_local_get_ex
bnz exist
int 0
==
bnz exit
exist:
err
exit:
int 1
==`

	pre, now, ledger := makeOldAndNewEnv(directRefEnabledVersion)
	ledger.NewAccount(now.TxnGroup[0].Txn.Receiver, 1)
	testApp(t, text, now, "invalid Account reference")

	text = `int 1  // account idx
int 100 // app id
txn ApplicationArgs 0
app_local_get_ex
bnz exist
int 0
==
bnz exit
exist:
err
exit:
int 1`

	testApp(t, text, now, "is not opted into")

	// Make a different app (not 100)
	ledger.NewApp(now.TxnGroup[0].Txn.Receiver, 9999, basics.AppParams{})
	testApp(t, text, now, "is not opted into")

	// create the app and check the value from ApplicationArgs[0] (protocol.PaymentTx) does not exist
	ledger.NewApp(now.TxnGroup[0].Txn.Receiver, 100, basics.AppParams{})
	ledger.NewLocals(now.TxnGroup[0].Txn.Receiver, 100)
	testApp(t, text, now)

	text = `int 1  // account idx
int 100 // app id
txn ApplicationArgs 0
app_local_get_ex
bnz exist
err
exist:
byte "ALGO"
==`
	ledger.NewLocal(now.TxnGroup[0].Txn.Receiver, 100, string(protocol.PaymentTx), basics.TealValue{Type: basics.TealBytesType, Bytes: "ALGO"})

	testApp(t, text, now)
	testApp(t, strings.Replace(text, "int 1  // account idx", "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui01\"", -1), now)
	testProg(t, strings.Replace(text, "int 1  // account idx", "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui01\"", -1), directRefEnabledVersion-1,
		Expect{4, "app_local_get_ex arg 0 wanted type uint64..."})
	testApp(t, strings.Replace(text, "int 100 // app id", "int 2", -1), now)
	// Next we're testing if the use of the current app's id works
	// as a direct reference. The error is because the receiver
	// account is not opted into 123.
	now.TxnGroup[0].Txn.ApplicationID = 123
	testApp(t, strings.Replace(text, "int 100 // app id", "int 123", -1), now, "is not opted into")
	testApp(t, strings.Replace(text, "int 100 // app id", "int 2", -1), pre, "is not opted into")
	testApp(t, strings.Replace(text, "int 100 // app id", "int 9", -1), now, "invalid App reference 9")
	testApp(t, strings.Replace(text, "int 1  // account idx", "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui00\"", -1), now,
		"no account")

	// opt into 123, and try again
	ledger.NewApp(now.TxnGroup[0].Txn.Receiver, 123, basics.AppParams{})
	ledger.NewLocals(now.TxnGroup[0].Txn.Receiver, 123)
	ledger.NewLocal(now.TxnGroup[0].Txn.Receiver, 123, string(protocol.PaymentTx), basics.TealValue{Type: basics.TealBytesType, Bytes: "ALGO"})
	testApp(t, strings.Replace(text, "int 100 // app id", "int 123", -1), now)
	testApp(t, strings.Replace(text, "int 100 // app id", "int 0", -1), now)

	// Somewhat surprising, but in `pre` when the app argument was expected to be
	// an actual app id (not an index in foreign apps), 0 was *still* treated
	// like current app.
	pre.TxnGroup[0].Txn.ApplicationID = 123
	testApp(t, strings.Replace(text, "int 100 // app id", "int 0", -1), pre)

	// check special case account idx == 0 => sender
	ledger.NewApp(now.TxnGroup[0].Txn.Sender, 100, basics.AppParams{})
	ledger.NewLocals(now.TxnGroup[0].Txn.Sender, 100)
	text = `int 0  // account idx
int 100 // app id
txn ApplicationArgs 0
app_local_get_ex
bnz exist
err
exist:
byte "ALGO"
==`

	ledger.NewLocal(now.TxnGroup[0].Txn.Sender, 100, string(protocol.PaymentTx), basics.TealValue{Type: basics.TealBytesType, Bytes: "ALGO"})
	testApp(t, text, now)
	testApp(t, strings.Replace(text, "int 0  // account idx", "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui00\"", -1), now)
	testApp(t, strings.Replace(text, "int 0  // account idx", "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui02\"", -1), now,
		"invalid Account reference")

	// check reading state of other app
	ledger.NewApp(now.TxnGroup[0].Txn.Sender, 56, basics.AppParams{})
	ledger.NewApp(now.TxnGroup[0].Txn.Sender, 100, basics.AppParams{})
	text = `int 0  // account idx
int 56 // app id
txn ApplicationArgs 0
app_local_get_ex
bnz exist
err
exist:
byte "ALGO"
==`

	ledger.NewLocals(now.TxnGroup[0].Txn.Sender, 56)
	ledger.NewLocal(now.TxnGroup[0].Txn.Sender, 56, string(protocol.PaymentTx), basics.TealValue{Type: basics.TealBytesType, Bytes: "ALGO"})
	testApp(t, text, now)

	// check app_local_get
	text = `int 0  // account idx
txn ApplicationArgs 0
app_local_get
byte "ALGO"
==`

	ledger.NewLocal(now.TxnGroup[0].Txn.Sender, 100, string(protocol.PaymentTx), basics.TealValue{Type: basics.TealBytesType, Bytes: "ALGO"})
	now.TxnGroup[0].Txn.ApplicationID = 100
	testApp(t, text, now)
	testApp(t, strings.Replace(text, "int 0  // account idx", "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui00\"", -1), now)
	testProg(t, strings.Replace(text, "int 0  // account idx", "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui00\"", -1), directRefEnabledVersion-1,
		Expect{3, "app_local_get arg 0 wanted type uint64..."})
	testApp(t, strings.Replace(text, "int 0  // account idx", "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui01\"", -1), now)
	testApp(t, strings.Replace(text, "int 0  // account idx", "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui02\"", -1), now,
		"invalid Account reference")

	// check app_local_get default value
	text = `int 0  // account idx
byte "ALGO"
app_local_get
int 0
==`

	ledger.NewLocal(now.TxnGroup[0].Txn.Sender, 100, string(protocol.PaymentTx), basics.TealValue{Type: basics.TealBytesType, Bytes: "ALGO"})
	testApp(t, text, now)
}

func TestAppReadGlobalState(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	text := `int 0
txn ApplicationArgs 0
app_global_get_ex
bnz exist
err
exist:
byte "ALGO"
==
int 1  // ForeignApps index
txn ApplicationArgs 0
app_global_get_ex
bnz exist1
err
exist1:
byte "ALGO"
==
&&
txn ApplicationArgs 0
app_global_get
byte "ALGO"
==
&&
`
	pre, now, ledger := makeOldAndNewEnv(directRefEnabledVersion)
	ledger.NewAccount(now.TxnGroup[0].Txn.Sender, 1)

	now.TxnGroup[0].Txn.ApplicationID = 100
	now.TxnGroup[0].Txn.ForeignApps = []basics.AppIndex{now.TxnGroup[0].Txn.ApplicationID}
	testApp(t, text, now, "no app 100")

	// create the app and check the value from ApplicationArgs[0] (protocol.PaymentTx) does not exist
	ledger.NewApp(now.TxnGroup[0].Txn.Sender, 100, basics.AppParams{})

	testApp(t, text, now, "err opcode")

	ledger.NewGlobal(100, string(protocol.PaymentTx), basics.TealValue{Type: basics.TealBytesType, Bytes: "ALGO"})

	testApp(t, text, now)

	// check error on invalid app index for app_global_get_ex
	text = "int 2; txn ApplicationArgs 0; app_global_get_ex"
	testApp(t, text, now, "invalid App reference 2")
	// check that actual app id ok instead of indirect reference
	text = `int 100; txn ApplicationArgs 0; app_global_get_ex; int 1; ==; assert; byte "ALGO"; ==`
	testApp(t, text, now)
	testApp(t, text, pre, "invalid App reference 100") // but not in old teal

	// check app_global_get default value
	text = "byte 0x414c474f55; app_global_get; int 0; =="

	ledger.NewLocals(now.TxnGroup[0].Txn.Sender, 100)
	ledger.NewLocal(now.TxnGroup[0].Txn.Sender, 100, string(protocol.PaymentTx), basics.TealValue{Type: basics.TealBytesType, Bytes: "ALGO"})
	testApp(t, text, now)

	text = `
byte 0x41414141
int 4141
app_global_put
int 1  // ForeignApps index
byte 0x41414141
app_global_get_ex
bnz exist
err
exist:
int 4141
==
`
	// check that even during application creation (Txn.ApplicationID == 0)
	// we will use the the kvCow if the exact application ID (100) is
	// specified in the transaction
	now.TxnGroup[0].Txn.ApplicationID = 0
	now.TxnGroup[0].Txn.ForeignApps = []basics.AppIndex{100}

	testAppFull(t, testProg(t, text, directRefEnabledVersion).Program, 0, 100, now)

	// Direct reference to the current app also works
	now.TxnGroup[0].Txn.ForeignApps = []basics.AppIndex{}
	testAppFull(t, testProg(t, strings.Replace(text, "int 1  // ForeignApps index", "int 100", -1), directRefEnabledVersion).Program,
		0, 100, now)
	testAppFull(t, testProg(t, strings.Replace(text, "int 1  // ForeignApps index", "global CurrentApplicationID", -1), directRefEnabledVersion).Program,
		0, 100, now)
}

const assetsTestTemplate = `int 0//account
int 55
asset_holding_get AssetBalance
!
bnz error
int 123
==
int 0//account
int 55
asset_holding_get AssetFrozen
!
bnz error
int 1
==
&&
int 0//params
asset_params_get AssetTotal
!
bnz error
int 1000
==
&&
int 0//params
asset_params_get AssetDecimals
!
bnz error
int 2
==
&&
int 0//params
asset_params_get AssetDefaultFrozen
!
bnz error
int 0
==
&&
int 0//params
asset_params_get AssetUnitName
!
bnz error
byte "ALGO"
==
&&
int 0//params
asset_params_get AssetName
!
bnz error
len
int 0
==
&&
int 0//params
asset_params_get AssetURL
!
bnz error
txna ApplicationArgs 0
==
&&
int 0//params
asset_params_get AssetMetadataHash
!
bnz error
byte 0x0000000000000000000000000000000000000000000000000000000000000000
==
&&
int 0//params
asset_params_get AssetManager
!
bnz error
txna Accounts 0
==
&&
int 0//params
asset_params_get AssetReserve
!
bnz error
txna Accounts 1
==
&&
int 0//params
asset_params_get AssetFreeze
!
bnz error
txna Accounts 1
==
&&
int 0//params
asset_params_get AssetClawback
!
bnz error
txna Accounts 1
==
&&
bnz ok
error:
err
ok:
%s
int 1
`

const v5extras = `
int 0//params
asset_params_get AssetCreator
pop
txn Sender
==
assert
`

func TestAssets(t *testing.T) {
	partitiontest.PartitionTest(t)

	t.Parallel()
	tests := map[uint64]string{
		4: fmt.Sprintf(assetsTestTemplate, ""),
		5: fmt.Sprintf(assetsTestTemplate, v5extras),
	}

	for v, source := range tests {
		testAssetsByVersion(t, source, v)
	}
}

func testAssetsByVersion(t *testing.T, assetsTestProgram string, version uint64) {
	for _, field := range assetHoldingFieldNames {
		fs := assetHoldingFieldSpecByName[field]
		if fs.version <= version && !strings.Contains(assetsTestProgram, field) {
			t.Errorf("TestAssets missing field %v", field)
		}
	}
	for _, field := range assetParamsFieldNames {
		fs := assetParamsFieldSpecByName[field]
		if fs.version <= version && !strings.Contains(assetsTestProgram, field) {
			t.Errorf("TestAssets missing field %v", field)
		}
	}

	txn := makeSampleAppl(888)
	pre := defaultEvalParamsWithVersion(directRefEnabledVersion-1, txn)
	require.GreaterOrEqual(t, version, uint64(directRefEnabledVersion))
	now := defaultEvalParamsWithVersion(version, txn)
	ledger := NewLedger(
		map[basics.Address]uint64{
			txn.Txn.Sender: 1,
		},
	)
	pre.Ledger = ledger
	now.Ledger = ledger

	// bear in mind: the sample transaction has ForeignAccounts{55,77}
	testApp(t, "int 5; int 55; asset_holding_get AssetBalance", now, "invalid Account reference 5")
	// was legal to get balance on a non-ForeignAsset
	testApp(t, "int 0; int 54; asset_holding_get AssetBalance; ==", pre)
	// but not since directRefEnabledVersion
	testApp(t, "int 0; int 54; asset_holding_get AssetBalance", now, "invalid Asset reference 54")

	// it wasn't legal to use a direct ref for account
	testProg(t, `byte "aoeuiaoeuiaoeuiaoeuiaoeuiaoeui00"; int 54; asset_holding_get AssetBalance`,
		directRefEnabledVersion-1, Expect{1, "asset_holding_get AssetBalance arg 0 wanted type uint64..."})
	// but it is now (empty asset yields 0,0 on stack)
	testApp(t, `byte "aoeuiaoeuiaoeuiaoeuiaoeuiaoeui00"; int 55; asset_holding_get AssetBalance; ==`, now)
	// This is receiver, who is in Assets array
	testApp(t, `byte "aoeuiaoeuiaoeuiaoeuiaoeuiaoeui01"; int 55; asset_holding_get AssetBalance; ==`, now)
	// But this is not in Assets, so illegal
	testApp(t, `byte "aoeuiaoeuiaoeuiaoeuiaoeuiaoeui02"; int 55; asset_holding_get AssetBalance; ==`, now, "invalid")

	// for params get, presence in ForeignAssets has always be required
	testApp(t, "int 5; asset_params_get AssetTotal", pre, "invalid Asset reference 5")
	testApp(t, "int 5; asset_params_get AssetTotal", now, "invalid Asset reference 5")

	params := basics.AssetParams{
		Total:         1000,
		Decimals:      2,
		DefaultFrozen: false,
		UnitName:      "ALGO",
		AssetName:     "",
		URL:           string(protocol.PaymentTx),
		Manager:       txn.Txn.Sender,
		Reserve:       txn.Txn.Receiver,
		Freeze:        txn.Txn.Receiver,
		Clawback:      txn.Txn.Receiver,
	}

	ledger.NewAsset(txn.Txn.Sender, 55, params)
	ledger.NewHolding(txn.Txn.Sender, 55, 123, true)
	// For consistency you can now use an indirect ref in holding_get
	// (recall ForeignAssets[0] = 55, which has balance 123)
	testApp(t, "int 0; int 0; asset_holding_get AssetBalance; int 1; ==; assert; int 123; ==", now)
	// but previous code would still try to read ASA 0
	testApp(t, "int 0; int 0; asset_holding_get AssetBalance; int 0; ==; assert; int 0; ==", pre)

	testApp(t, assetsTestProgram, now)

	// In current versions, can swap out the account index for the account
	testApp(t, strings.Replace(assetsTestProgram, "int 0//account", "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui00\"", -1), now)
	// Or an asset index for the asset id
	testApp(t, strings.Replace(assetsTestProgram, "int 0//params", "int 55", -1), now)
	// Or an index for the asset id
	testApp(t, strings.Replace(assetsTestProgram, "int 55", "int 0", -1), now)

	// but old code cannot
	testProg(t, strings.Replace(assetsTestProgram, "int 0//account", "byte \"aoeuiaoeuiaoeuiaoeuiaoeuiaoeui00\"", -1), directRefEnabledVersion-1, Expect{3, "asset_holding_get AssetBalance arg 0 wanted type uint64..."})

	if version < 5 {
		// Can't run these with AppCreator anyway
		testApp(t, strings.Replace(assetsTestProgram, "int 0//params", "int 55", -1), pre, "invalid Asset ref")
		testApp(t, strings.Replace(assetsTestProgram, "int 55", "int 0", -1), pre, "err opcode")
	}

	// check holdings bool value
	source := `intcblock 0 55 1
intc_0  // 0, account idx (txn.Sender)
intc_1  // 55
asset_holding_get AssetFrozen
!
bnz error
intc_0 // 0
==
bnz ok
error:
err
ok:
intc_2 // 1
`
	ledger.NewHolding(txn.Txn.Sender, 55, 123, false)
	testApp(t, source, now)

	// check holdings invalid offsets
	ops := testProg(t, source, version)
	require.Equal(t, OpsByName[now.Proto.LogicSigVersion]["asset_holding_get"].Opcode, ops.Program[8])
	ops.Program[9] = 0x02
	_, err := EvalApp(ops.Program, 0, 888, now)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid asset_holding_get field 2")

	// check holdings bool value
	source = `intcblock 0 1
intc_0
asset_params_get AssetDefaultFrozen
!
bnz error
intc_1
==
bnz ok
error:
err
ok:
intc_1
`
	params.DefaultFrozen = true
	ledger.NewAsset(txn.Txn.Sender, 55, params)
	testApp(t, source, now)
	// check holdings invalid offsets
	ops = testProg(t, source, version)
	require.Equal(t, OpsByName[now.Proto.LogicSigVersion]["asset_params_get"].Opcode, ops.Program[6])
	ops.Program[7] = 0x20
	_, err = EvalApp(ops.Program, 0, 888, now)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid asset_params_get field 32")

	// check empty string
	source = `intcblock 0 1
intc_0  // foreign asset idx (txn.ForeignAssets[0])
asset_params_get AssetURL
!
bnz error
len
intc_0
==
bnz ok
error:
err
ok:
intc_1
`
	params.URL = ""
	ledger.NewAsset(txn.Txn.Sender, 55, params)
	testApp(t, source, now)

	source = `intcblock 1 9
intc_0  // foreign asset idx (txn.ForeignAssets[1])
asset_params_get AssetURL
!
bnz error
len
intc_1
==
bnz ok
error:
err
ok:
intc_0
`
	params.URL = "foobarbaz"
	ledger.NewAsset(txn.Txn.Sender, 77, params)
	testApp(t, source, now)

	source = `intcblock 0 1
intc_0
asset_params_get AssetURL
!
bnz error
intc_0
==
bnz ok
error:
err
ok:
intc_1
`
	params.URL = ""
	ledger.NewAsset(txn.Txn.Sender, 55, params)
	testApp(t, notrack(source), now, "cannot compare ([]byte to uint64)")
}

// TestAssetDisambiguation ensures we have a consistent interpretation of low
// numbers when used as an argument to asset_*_get. A low number is an asset ID
// if that asset ID is available, or a slot number in txn.Assets if not.
func TestAssetDisambiguation(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	// start at 4 when the two meanings were added, stop at 8 because 9 removed the dual meaning
	testLogicRange(t, 4, 8, func(t *testing.T, ep *EvalParams, tx *transactions.Transaction, ledger *Ledger) {
		ledger.NewAsset(tx.Sender, 1, basics.AssetParams{AssetName: "one", Total: 1})
		ledger.NewAsset(tx.Sender, 20, basics.AssetParams{AssetName: "twenty", Total: 20})
		ledger.NewAsset(tx.Sender, 30, basics.AssetParams{AssetName: "thirty", Total: 30})
		tx.ForeignAssets = []basics.AssetIndex{20, 30}
		// Since 1 is not available, 1 must mean the 1th asset slot = 30
		testApp(t, `int 1; asset_params_get AssetName; assert; byte "thirty"; ==`, ep)
		testApp(t, `int 0; int 1; asset_holding_get AssetBalance; assert; int 30; ==`, ep)

		tx.ForeignAssets = []basics.AssetIndex{1, 30}
		// Since 1 IS available, 1 means the assetid=1, not the 1th slot
		testApp(t, `int 1; asset_params_get AssetName; assert; byte "one"; ==`, ep)
		testApp(t, `int 0; int 1; asset_holding_get AssetBalance; assert; int 1; ==`, ep)
	})
}

// TestAppDisambiguation ensures we have a consistent interpretation of low
// numbers when used as an argument to app_(global,local)_get. A low number is
// an app ID if that app ID is available, or a slot number in
// txn.ForeignApplications if not.
func TestAppDisambiguation(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	// start at 4 when the two meanings were added, stop at 8 because 9 removed the dual meaning
	testLogicRange(t, 4, 8, func(t *testing.T, ep *EvalParams, tx *transactions.Transaction, ledger *Ledger) {
		ledger.NewApp(tx.Sender, 1, basics.AppParams{
			GlobalState: map[string]basics.TealValue{"a": {
				Type: basics.TealUintType,
				Uint: 1,
			}},
			ExtraProgramPages: 1,
		})
		ledger.NewLocals(tx.Sender, 1)
		ledger.NewLocal(tx.Sender, 1, "x", basics.TealValue{Type: basics.TealUintType, Uint: 100})
		ledger.NewApp(tx.Sender, 20, basics.AppParams{
			GlobalState: map[string]basics.TealValue{"a": {
				Type: basics.TealUintType,
				Uint: 20,
			}},
			ExtraProgramPages: 20,
		})
		ledger.NewLocals(tx.Sender, 20)
		ledger.NewLocal(tx.Sender, 20, "x", basics.TealValue{Type: basics.TealUintType, Uint: 200})
		ledger.NewApp(tx.Sender, 30, basics.AppParams{
			GlobalState: map[string]basics.TealValue{"a": {
				Type: basics.TealUintType,
				Uint: 30,
			}},
			ExtraProgramPages: 30,
		})
		tx.ForeignApps = []basics.AppIndex{20, 30}
		// Since 1 is not available, 1 must mean the first app slot = 20 (recall, 0 mean "this app")
		if ep.Proto.LogicSigVersion >= 5 {
			testApp(t, `int 1; app_params_get AppExtraProgramPages; assert; int 20; ==`, ep)
		}
		testApp(t, `int 1; byte "a"; app_global_get_ex; assert; int 20; ==`, ep)
		testApp(t, `int 0; int 1; byte "x"; app_local_get_ex; assert; int 200; ==`, ep)

		tx.ForeignApps = []basics.AppIndex{1, 30}
		// Since 1 IS available, 1 means the assetid=1, not the 1th slot
		if ep.Proto.LogicSigVersion >= 5 {
			testApp(t, `int 1; app_params_get AppExtraProgramPages; assert; int 1; ==`, ep)
		}
		testApp(t, `int 1; byte "a"; app_global_get_ex; assert; int 1; ==`, ep)
		testApp(t, `int 0; int 1; byte "x"; app_local_get_ex; assert; int 100; ==`, ep)
	})
}

func TestAppParams(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()
	// start at 5 for app_params_get
	testLogicRange(t, 5, 0, func(t *testing.T, ep *EvalParams, tx *transactions.Transaction, ledger *Ledger) {
		ledger.NewAccount(tx.Sender, 1)
		ledger.NewApp(tx.Sender, 100, basics.AppParams{})

		/* app id is in ForeignApps, but does not exist */
		source := "int 56; app_params_get AppExtraProgramPages; int 0; ==; assert; int 0; =="
		testApp(t, source, ep)
		/* app id is in ForeignApps, but has zero ExtraProgramPages */
		source = "int 100; app_params_get AppExtraProgramPages; int 1; ==; assert; int 0; =="
		testApp(t, source, ep)
	})
}

func TestAcctParams(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()
	ep, tx, ledger := makeSampleEnv()

	source := "txn Sender; acct_params_get AcctBalance; !; assert; int 0; =="
	testApp(t, source, ep)

	source = "txn Sender; acct_params_get AcctMinBalance; !; assert; int 1001; =="
	testApp(t, source, ep)

	ledger.NewAccount(tx.Sender, 42)

	source = "txn Sender; acct_params_get AcctBalance; assert; int 42; =="
	testApp(t, source, ep)

	source = "txn Sender; acct_params_get AcctMinBalance; assert; int 1001; =="
	testApp(t, source, ep)

	source = "txn Sender; acct_params_get AcctAuthAddr; assert; global ZeroAddress; =="
	testApp(t, source, ep)

	// No apps or schema at first, then 1 created and the global schema noted
	source = "txn Sender; acct_params_get AcctTotalAppsCreated; assert; !"
	testApp(t, source, ep)
	source = "txn Sender; acct_params_get AcctTotalNumUint; assert; !"
	testApp(t, source, ep)
	source = "txn Sender; acct_params_get AcctTotalNumByteSlice; assert; !"
	testApp(t, source, ep)
	source = "txn Sender; acct_params_get AcctTotalExtraAppPages; assert; !"
	testApp(t, source, ep)
	ledger.NewApp(tx.Sender, 2000, basics.AppParams{
		StateSchemas: basics.StateSchemas{
			LocalStateSchema: basics.StateSchema{
				NumUint:      6,
				NumByteSlice: 7,
			},
			GlobalStateSchema: basics.StateSchema{
				NumUint:      8,
				NumByteSlice: 9,
			},
		},
		ExtraProgramPages: 2,
	})
	source = "txn Sender; acct_params_get AcctTotalAppsCreated; assert; int 1; =="
	testApp(t, source, ep)
	source = "txn Sender; acct_params_get AcctTotalNumUint; assert; int 8; =="
	testApp(t, source, ep)
	source = "txn Sender; acct_params_get AcctTotalNumByteSlice; assert; int 9; =="
	testApp(t, source, ep)
	source = "txn Sender; acct_params_get AcctTotalExtraAppPages; assert; int 2; =="
	testApp(t, source, ep)

	// Not opted in at first, then opted into 1, schema added
	source = "txn Sender; acct_params_get AcctTotalAppsOptedIn; assert; !"
	testApp(t, source, ep)
	ledger.NewLocals(tx.Sender, 2000)
	source = "txn Sender; acct_params_get AcctTotalAppsOptedIn; assert; int 1; =="
	testApp(t, source, ep)
	source = "txn Sender; acct_params_get AcctTotalNumUint; assert; int 8; int 6; +; =="
	testApp(t, source, ep)
	source = "txn Sender; acct_params_get AcctTotalNumByteSlice; assert; int 9; int 7; +; =="
	testApp(t, source, ep)

	// No ASAs at first, then 1 created AND in total
	source = "txn Sender; acct_params_get AcctTotalAssetsCreated; assert; !"
	testApp(t, source, ep)
	source = "txn Sender; acct_params_get AcctTotalAssets; assert; !"
	testApp(t, source, ep)
	ledger.NewAsset(tx.Sender, 3000, basics.AssetParams{})
	source = "txn Sender; acct_params_get AcctTotalAssetsCreated; assert; int 1; =="
	testApp(t, source, ep)
	source = "txn Sender; acct_params_get AcctTotalAssets; assert; int 1; =="
	testApp(t, source, ep)
}

// TestGlobalNonDelete ensures that a deletion is not inserted in the delta if the global didn't exist
func TestGlobalNonDelete(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	ep, txn, ledger := makeSampleEnv()
	source := `
byte "none"
app_global_del
int 1
`
	ledger.NewApp(txn.Sender, 888, makeApp(0, 0, 1, 0))
	delta := testApp(t, source, ep)
	require.Empty(t, delta.GlobalDelta)
	require.Empty(t, delta.LocalDeltas)
}

// TestLocalNonDelete ensures that a deletion is not inserted in the delta if the local didn't exist
func TestLocalNonDelete(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	ep, txn, ledger := makeSampleEnv()
	source := `
txn Sender
byte "none"
app_local_del
int 1
`
	ledger.NewAccount(txn.Sender, 100000)
	ledger.NewApp(txn.Sender, 888, makeApp(0, 0, 1, 0))
	ledger.NewLocals(txn.Sender, 888)
	delta := testApp(t, source, ep)
	require.Empty(t, delta.GlobalDelta)
	require.Empty(t, delta.LocalDeltas)
}

func TestAppLocalReadWriteDeleteErrors(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	sourceRead := `intcblock 0 100 0x77 1
bytecblock "ALGO" "ALGOA"
txn Sender
intc_1                    // 100, app id
bytec_0                   // key "ALGO"
app_local_get_ex
!
bnz error
intc_2                    // 0x77
==
txn Sender
intc_1                    // 100
bytec_1                   // ALGOA
app_local_get_ex
!
bnz error
intc_3                    // 1
==
&&
bnz ok
error:
err
ok:
intc_3                    // 1
`
	sourceWrite := `intcblock 0 100 1
bytecblock "ALGO"
txn Sender
bytec_0                    // key "ALGO"
intc_1                     // 100
app_local_put
intc_2                     // 1
`
	sourceDelete := `intcblock 0 100
bytecblock "ALGO"
txn Sender
bytec_0                      // key "ALGO"
app_local_del
intc_1
`
	type cmdtest struct {
		source string
	}

	tests := map[string]cmdtest{
		"read":   {sourceRead},
		"write":  {sourceWrite},
		"delete": {sourceDelete},
	}
	for name, cmdtest := range tests {
		name, cmdtest := name, cmdtest
		t.Run(fmt.Sprintf("test=%s", name), func(t *testing.T) {
			t.Parallel()
			source := cmdtest.source

			ops := testProg(t, source, AssemblerMaxVersion)

			var txn transactions.SignedTxn
			txn.Txn.Type = protocol.ApplicationCallTx
			txn.Txn.ApplicationID = 100
			ep := defaultEvalParams(txn)
			err := CheckContract(ops.Program, ep)
			require.NoError(t, err)

			ledger := NewLedger(
				map[basics.Address]uint64{
					txn.Txn.Sender: 1,
				},
			)
			ep.Ledger = ledger
			ep.SigLedger = ledger

			_, err = EvalApp(ops.Program, 0, 100, ep)
			require.Error(t, err)
			require.Contains(t, err.Error(), "is not opted into")

			ledger.NewApp(txn.Txn.Sender, 100, basics.AppParams{})
			ledger.NewLocals(txn.Txn.Sender, 100)

			if name == "read" {
				_, err = EvalApp(ops.Program, 0, 100, ep)
				require.Error(t, err)
				require.Contains(t, err.Error(), "err opcode") // no such key
			}

			ledger.NewLocal(txn.Txn.Sender, 100, "ALGO", basics.TealValue{Type: basics.TealUintType, Uint: 0x77})
			ledger.NewLocal(txn.Txn.Sender, 100, "ALGOA", basics.TealValue{Type: basics.TealUintType, Uint: 1})

			ledger.Reset()
			pass, err := EvalApp(ops.Program, 0, 100, ep)
			require.NoError(t, err)
			require.True(t, pass)
			delta := ep.TxnGroup[0].EvalDelta
			require.Empty(t, delta.GlobalDelta)
			expLocal := 1
			if name == "read" {
				expLocal = 0
			}
			require.Len(t, delta.LocalDeltas, expLocal)
		})
	}
}

func TestAppLocalStateReadWrite(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	testLogicRange(t, 2, 0, func(t *testing.T, ep *EvalParams, txn *transactions.Transaction, ledger *Ledger) {

		txn.ApplicationID = 100
		ledger.NewAccount(txn.Sender, 1)
		ledger.NewApp(txn.Sender, 100, basics.AppParams{})
		ledger.NewLocals(txn.Sender, 100)

		// write int and bytes values
		source := `txn Sender
byte "ALGO"      // key
int 0x77             // value
app_local_put
txn Sender
byte "ALGOA"    // key
byte "ALGO"      // value
app_local_put
txn Sender
int 100              // app id
byte "ALGOA"    // key
app_local_get_ex
bnz exist
err
exist:
byte "ALGO"
==
txn Sender
int 100              // app id
byte "ALGO"      // key
app_local_get_ex
bnz exist2
err
exist2:
int 0x77
==
&&
`
		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
		}
		delta := testApp(t, source, ep)
		require.Empty(t, delta.GlobalDelta)
		require.Len(t, delta.LocalDeltas, 1)

		require.Len(t, delta.LocalDeltas[0], 2)
		vd := delta.LocalDeltas[0]["ALGO"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x77), vd.Uint)

		vd = delta.LocalDeltas[0]["ALGOA"]
		require.Equal(t, basics.SetBytesAction, vd.Action)
		require.Equal(t, "ALGO", vd.Bytes)

		// write same value without writing, expect no local delta
		source = `txn Sender
byte "ALGO"       // key
int 0x77              // value
app_local_put
txn Sender
int 100               // app id
byte "ALGO"       // key
app_local_get_ex
bnz exist
err
exist:
int 0x77
==
`
		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
		}
		ledger.Reset()
		ledger.NoLocal(txn.Sender, 100, "ALGOA")
		ledger.NoLocal(txn.Sender, 100, "ALGO")

		algoValue := basics.TealValue{Type: basics.TealUintType, Uint: 0x77}
		ledger.NewLocal(txn.Sender, 100, "ALGO", algoValue)

		delta = testApp(t, source, ep)
		require.Empty(t, delta.GlobalDelta)
		require.Empty(t, delta.LocalDeltas)

		// write same value after reading, expect no local delta
		source = `txn Sender
int 100              // app id
byte "ALGO"      // key
app_local_get_ex
bnz exist
err
exist:
txn Sender
byte "ALGO"      // key
int 0x77             // value
app_local_put
txn Sender
int 100              // app id
byte "ALGO"      // key
app_local_get_ex
bnz exist2
err
exist2:
==
`
		ledger.Reset()
		ledger.NewLocal(txn.Sender, 100, "ALGO", algoValue)
		ledger.NoLocal(txn.Sender, 100, "ALGOA")

		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
		}
		delta = testApp(t, source, ep)
		require.Empty(t, delta.GlobalDelta)
		require.Empty(t, delta.LocalDeltas)

		// write a value and expect local delta change
		source = `txn Sender
byte "ALGOA"    // key
int 0x78        // value
app_local_put
int 1
`
		ledger.Reset()
		ledger.NewLocal(txn.Sender, 100, "ALGO", algoValue)
		ledger.NoLocal(txn.Sender, 100, "ALGOA")

		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
		}
		delta = testApp(t, source, ep)
		require.Empty(t, delta.GlobalDelta)
		require.Len(t, delta.LocalDeltas, 1)
		require.Len(t, delta.LocalDeltas[0], 1)
		vd = delta.LocalDeltas[0]["ALGOA"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x78), vd.Uint)

		// write a value to existing key and expect delta change and reading the new value
		source = `txn Sender
byte "ALGO"          // key
int 0x78             // value
app_local_put
txn Sender
int 100              // app id
byte "ALGO"          // key
app_local_get_ex
bnz exist
err
exist:
int 0x78
==
`
		ledger.Reset()
		ledger.NewLocal(txn.Sender, 100, "ALGO", algoValue)
		ledger.NoLocal(txn.Sender, 100, "ALGOA")

		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
		}
		delta = testApp(t, source, ep)
		require.Empty(t, delta.GlobalDelta)
		require.Len(t, delta.LocalDeltas, 1)
		require.Len(t, delta.LocalDeltas[0], 1)
		vd = delta.LocalDeltas[0]["ALGO"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x78), vd.Uint)

		// write a value after read and expect delta change
		source = `txn Sender
int 100              // app id
byte "ALGO"          // key
app_local_get_ex
bnz exist
err
exist:
txn Sender
byte "ALGO"          // key
int 0x78             // value
app_local_put
`
		ledger.Reset()
		ledger.NewLocal(txn.Sender, 100, "ALGO", algoValue)
		ledger.NoLocal(txn.Sender, 100, "ALGOA")

		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
		}
		delta = testApp(t, source, ep)
		require.Empty(t, delta.GlobalDelta)
		require.Len(t, delta.LocalDeltas, 1)
		require.Len(t, delta.LocalDeltas[0], 1)
		vd = delta.LocalDeltas[0]["ALGO"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x78), vd.Uint)

		// write a few values and expect delta change only for unique changed
		source = `txn Sender
byte "ALGO"          // key
int 0x77             // value
app_local_put
txn Sender
byte "ALGO"          // key
int 0x78             // value
app_local_put
txn Sender
byte "ALGOA"           // key
int 0x78             // value
app_local_put
txn Accounts 1
byte "ALGO"          // key
int 0x79             // value
app_local_put
int 1
`
		ledger.Reset()
		ledger.NewLocal(txn.Sender, 100, "ALGO", algoValue)
		ledger.NoLocal(txn.Sender, 100, "ALGOA")

		ledger.NewAccount(txn.Receiver, 500)
		ledger.NewLocals(txn.Receiver, 100)

		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
			source = strings.ReplaceAll(source, "txn Accounts 1", "int 1")
		}
		delta = testApp(t, source, ep)
		require.Empty(t, delta.GlobalDelta)
		require.Len(t, delta.LocalDeltas, 2)
		require.Len(t, delta.LocalDeltas[0], 2)
		require.Len(t, delta.LocalDeltas[1], 1)
		vd = delta.LocalDeltas[0]["ALGO"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x78), vd.Uint)

		vd = delta.LocalDeltas[0]["ALGOA"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x78), vd.Uint)

		vd = delta.LocalDeltas[1]["ALGO"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x79), vd.Uint)
	})
}

func TestAppLocalGlobalErrorCases(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	ep, tx, ledger := makeSampleEnv()
	ledger.NewApp(tx.Sender, 888, basics.AppParams{})

	testApp(t, fmt.Sprintf(`byte "%v"; int 1; app_global_put; int 1`, strings.Repeat("v", ep.Proto.MaxAppKeyLen+1)), ep, "key too long")

	testApp(t, fmt.Sprintf(`byte "%v"; int 1; app_global_put; int 1`, strings.Repeat("v", ep.Proto.MaxAppKeyLen)), ep)

	ledger.NewLocals(tx.Sender, 888)
	testApp(t, fmt.Sprintf(`txn Sender; byte "%v"; int 1; app_local_put; int 1`, strings.Repeat("v", ep.Proto.MaxAppKeyLen+1)), ep, "key too long")

	testApp(t, fmt.Sprintf(`txn Sender; byte "%v"; int 1; app_local_put; int 1`, strings.Repeat("v", ep.Proto.MaxAppKeyLen)), ep)

	testApp(t, fmt.Sprintf(`byte "foo"; byte "%v"; app_global_put; int 1`, strings.Repeat("v", ep.Proto.MaxAppBytesValueLen+1)), ep, "value too long for key")

	testApp(t, fmt.Sprintf(`byte "foo"; byte "%v"; app_global_put; int 1`, strings.Repeat("v", ep.Proto.MaxAppBytesValueLen)), ep)

	testApp(t, fmt.Sprintf(`txn Sender; byte "foo"; byte "%v"; app_local_put; int 1`, strings.Repeat("v", ep.Proto.MaxAppBytesValueLen+1)), ep, "value too long for key")

	testApp(t, fmt.Sprintf(`txn Sender; byte "foo"; byte "%v"; app_local_put; int 1`, strings.Repeat("v", ep.Proto.MaxAppBytesValueLen)), ep)

	ep.Proto.MaxAppSumKeyValueLens = 2 // Override to generate error.
	testApp(t, `byte "foo"; byte "foo"; app_global_put; int 1`, ep, "key/value total too long for key")

	testApp(t, `txn Sender; byte "foo"; byte "foo"; app_local_put; int 1`, ep, "key/value total too long for key")
}

func TestAppGlobalReadWriteDeleteErrors(t *testing.T) {
	partitiontest.PartitionTest(t)

	t.Parallel()

	sourceRead := `int 0
byte "ALGO"  // key
app_global_get_ex
bnz ok
err
ok:
int 0x77
==
`
	sourceReadSimple := `byte "ALGO"  // key
app_global_get
int 0x77
==
`

	sourceWrite := `byte "ALGO"  // key
int 100
app_global_put
int 1
`
	sourceDelete := `byte "ALGO"  // key
app_global_del
int 1
`
	tests := map[string]string{
		"read":   sourceRead,
		"reads":  sourceReadSimple,
		"write":  sourceWrite,
		"delete": sourceDelete,
	}
	for name, source := range tests {
		name, source := name, source
		t.Run(fmt.Sprintf("test=%s", name), func(t *testing.T) {
			t.Parallel()
			ops, err := AssembleStringWithVersion(source, AssemblerMaxVersion)
			require.NoError(t, err)

			ep, txn, ledger := makeSampleEnv()
			txn.ApplicationID = basics.AppIndex(100)
			testAppBytes(t, ops.Program, ep, "no app 100")

			ledger.NewApp(txn.Sender, 100, makeApp(0, 0, 1, 0))

			// a special test for read
			if name == "read" {
				testAppBytes(t, ops.Program, ep, "err opcode") // no such key
			}
			ledger.NewGlobal(100, "ALGO", basics.TealValue{Type: basics.TealUintType, Uint: 0x77})

			ledger.Reset()

			delta := testAppBytes(t, ops.Program, ep)
			require.Empty(t, delta.LocalDeltas)
		})
	}
}

func TestAppGlobalReadWrite(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	testLogicRange(t, 2, 0, func(t *testing.T, ep *EvalParams, txn *transactions.Transaction, ledger *Ledger) {

		// check writing ints and bytes
		source := `byte "ALGO"  // key
int 0x77						// value
app_global_put
byte "ALGOA"  // key "ALGOA"
byte "ALGO"    // value
app_global_put
// check simple
byte "ALGOA"  // key "ALGOA"
app_global_get
byte "ALGO"
==
// check generic with alias
int 0 // current app id alias
byte "ALGOA"  // key "ALGOA"
app_global_get_ex
bnz ok
err
ok:
byte "ALGO"
==
&&
// check generic with exact app id
THISAPP
byte "ALGOA"  // key "ALGOA"
app_global_get_ex
bnz ok1
err
ok1:
byte "ALGO"
==
&&
// check simple
byte "ALGO"
app_global_get
int 0x77
==
&&
// check generic with alias
int 0 // ForeignApps index - current app
byte "ALGO"
app_global_get_ex
bnz ok2
err
ok2:
int 0x77
==
&&
// check generic with exact app id
THISAPP
byte "ALGO"
app_global_get_ex
bnz ok3
err
ok3:
int 0x77
==
&&
`

		txn.Type = protocol.ApplicationCallTx
		txn.ApplicationID = 100
		txn.ForeignApps = []basics.AppIndex{txn.ApplicationID}
		ledger.NewAccount(txn.Sender, 1)
		ledger.NewApp(txn.Sender, 100, basics.AppParams{})

		if ep.Proto.LogicSigVersion < sharedResourcesVersion {
			// 100 is in the ForeignApps array, name it by slot
			source = strings.ReplaceAll(source, "THISAPP", "int 1")
		} else {
			// use the actual app number, slots no longer allowed
			source = strings.ReplaceAll(source, "THISAPP", "int 100")
		}
		delta := testApp(t, source, ep)

		require.Len(t, delta.GlobalDelta, 2)
		require.Empty(t, delta.LocalDeltas)

		vd := delta.GlobalDelta["ALGO"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x77), vd.Uint)

		vd = delta.GlobalDelta["ALGOA"]
		require.Equal(t, basics.SetBytesAction, vd.Action)
		require.Equal(t, "ALGO", vd.Bytes)

		// write existing value before read
		source = `byte "ALGO"  // key
int 0x77						// value
app_global_put
byte "ALGO"
app_global_get
int 0x77
==
`
		ledger.Reset()
		ledger.NoGlobal(100, "ALGOA")
		ledger.NoGlobal(100, "ALGO")

		algoValue := basics.TealValue{Type: basics.TealUintType, Uint: 0x77}
		ledger.NewGlobal(100, "ALGO", algoValue)

		delta = testApp(t, source, ep)
		require.Empty(t, delta.GlobalDelta)
		require.Empty(t, delta.LocalDeltas)

		// write existing value after read
		source = `int 0
byte "ALGO"
app_global_get_ex
bnz ok
err
ok:
pop
byte "ALGO"
int 0x77
app_global_put
byte "ALGO"
app_global_get
int 0x77
==
`
		ledger.Reset()
		ledger.NoGlobal(100, "ALGOA")
		ledger.NewGlobal(100, "ALGO", algoValue)

		delta = testApp(t, source, ep)
		require.Empty(t, delta.GlobalDelta)
		require.Empty(t, delta.LocalDeltas)

		// write new values after and before read
		source = `int 0
byte "ALGO"
app_global_get_ex
bnz ok
err
ok:
pop
byte "ALGO"
int 0x78
app_global_put
int 0
byte "ALGO"
app_global_get_ex
bnz ok2
err
ok2:
int 0x78
==
byte "ALGOA"
byte "ALGO"
app_global_put
int 0
byte "ALGOA"
app_global_get_ex
bnz ok3
err
ok3:
byte "ALGO"
==
&&
`
		ledger.Reset()
		ledger.NoGlobal(100, "ALGOA")
		ledger.NewGlobal(100, "ALGO", algoValue)

		delta = testApp(t, source, ep)

		require.Len(t, delta.GlobalDelta, 2)
		require.Empty(t, delta.LocalDeltas)

		vd = delta.GlobalDelta["ALGO"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x78), vd.Uint)

		vd = delta.GlobalDelta["ALGOA"]
		require.Equal(t, basics.SetBytesAction, vd.Action)
		require.Equal(t, "ALGO", vd.Bytes)
	})
}

func TestAppGlobalReadOtherApp(t *testing.T) {
	partitiontest.PartitionTest(t)

	t.Parallel()
	// app_global_get_ex starts in v2
	testLogicRange(t, 2, 0, func(t *testing.T, ep *EvalParams, txn *transactions.Transaction, ledger *Ledger) {
		source := `
OTHERAPP
byte "mykey1"
app_global_get_ex
bz ok1
err
ok1:
pop
OTHERAPP
byte "mykey"
app_global_get_ex
bnz ok2
err
ok2:
byte "myval"
==
`

		if ep.Proto.LogicSigVersion < sharedResourcesVersion {
			// 101 is in the ForeignApps array, name it by slot
			source = strings.ReplaceAll(source, "OTHERAPP", "int 2")
		} else {
			// use the actual app number, slots no longer allowed
			source = strings.ReplaceAll(source, "OTHERAPP", "int 101")
		}

		txn.ApplicationID = 100
		txn.ForeignApps = []basics.AppIndex{txn.ApplicationID, 101}
		ledger.NewAccount(txn.Sender, 1)
		ledger.NewApp(txn.Sender, 100, basics.AppParams{})

		delta := testApp(t, source, ep, "no app 101")
		require.Empty(t, delta.GlobalDelta)
		require.Empty(t, delta.LocalDeltas)

		ledger.NewApp(txn.Receiver, 101, basics.AppParams{})
		ledger.NewApp(txn.Receiver, 100, basics.AppParams{}) // this keeps current app id = 100
		algoValue := basics.TealValue{Type: basics.TealBytesType, Bytes: "myval"}
		ledger.NewGlobal(101, "mykey", algoValue)

		delta = testApp(t, source, ep)
		require.Empty(t, delta.GlobalDelta)
		require.Empty(t, delta.LocalDeltas)
	})
}

func TestBlankKey(t *testing.T) {
	partitiontest.PartitionTest(t)

	t.Parallel()
	source := `
byte ""
app_global_get
int 0
==
assert

byte ""
int 7
app_global_put

byte ""
app_global_get
int 7
==
`
	txn := makeSampleAppl(100)
	ep := defaultEvalParams(txn)
	ledger := NewLedger(nil)
	ledger.NewAccount(txn.Txn.Sender, 1)
	ep.Ledger = ledger
	ep.SigLedger = ledger
	ledger.NewApp(txn.Txn.Sender, 100, basics.AppParams{})

	delta := testApp(t, source, ep)
	require.Empty(t, delta.LocalDeltas)
}

func TestAppGlobalDelete(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	testLogicRange(t, 2, 0, func(t *testing.T, ep *EvalParams, txn *transactions.Transaction, ledger *Ledger) {
		// check write/delete/read
		source := `byte "ALGO"
int 0x77						// value
app_global_put
byte "ALGOA"
byte "ALGO"
app_global_put
byte "ALGO"
app_global_del
byte "ALGOA"
app_global_del
int 0
byte "ALGO"
app_global_get_ex
bnz error
int 0
byte "ALGOA"
app_global_get_ex
bnz error
==
bnz ok
error:
err
ok:
int 1
`

		ledger.NewAccount(txn.Sender, 1)
		txn.ApplicationID = 100
		ledger.NewApp(txn.Sender, 100, basics.AppParams{})

		delta := testApp(t, source, ep)
		require.Len(t, delta.GlobalDelta, 2)
		require.Empty(t, delta.LocalDeltas)

		ledger.Reset()
		ledger.NoGlobal(100, "ALGOA")
		ledger.NoGlobal(100, "ALGO")

		algoValue := basics.TealValue{Type: basics.TealUintType, Uint: 0x77}
		ledger.NewGlobal(100, "ALGO", algoValue)

		// check delete existing
		source = `byte "ALGO"
app_global_del
THISAPP
byte "ALGO"
app_global_get_ex
==  // two zeros
`

		if ep.Proto.LogicSigVersion < sharedResourcesVersion {
			// 100 is in the ForeignApps array, name it by slot
			source = strings.ReplaceAll(source, "THISAPP", "int 1")
		} else {
			// use the actual app number, slots no longer allowed
			source = strings.ReplaceAll(source, "THISAPP", "int 100")
		}
		txn.ForeignApps = []basics.AppIndex{txn.ApplicationID}
		delta = testApp(t, source, ep)
		require.Len(t, delta.GlobalDelta, 1)
		vd := delta.GlobalDelta["ALGO"]
		require.Equal(t, basics.DeleteAction, vd.Action)
		require.Equal(t, uint64(0), vd.Uint)
		require.Equal(t, "", vd.Bytes)
		require.Equal(t, 0, len(delta.LocalDeltas))

		ledger.Reset()
		ledger.NoGlobal(100, "ALGOA")
		ledger.NoGlobal(100, "ALGO")

		ledger.NewGlobal(100, "ALGO", algoValue)

		// check delete and write non-existing
		source = `byte "ALGOA"
app_global_del
int 0
byte "ALGOA"
app_global_get_ex
==  // two zeros
byte "ALGOA"
int 0x78
app_global_put
`
		delta = testApp(t, source, ep)
		require.Len(t, delta.GlobalDelta, 1)
		vd = delta.GlobalDelta["ALGOA"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x78), vd.Uint)
		require.Equal(t, "", vd.Bytes)
		require.Empty(t, delta.LocalDeltas)

		ledger.Reset()
		ledger.NoGlobal(100, "ALGOA")
		ledger.NoGlobal(100, "ALGO")

		ledger.NewGlobal(100, "ALGO", algoValue)

		// check delete and write existing
		source = `byte "ALGO"
app_global_del
byte "ALGO"
int 0x78
app_global_put
int 1
`
		delta = testApp(t, source, ep)
		require.Len(t, delta.GlobalDelta, 1)
		vd = delta.GlobalDelta["ALGO"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Empty(t, delta.LocalDeltas)

		ledger.Reset()
		ledger.Reset()
		ledger.NoGlobal(100, "ALGOA")
		ledger.NoGlobal(100, "ALGO")

		ledger.NewGlobal(100, "ALGO", algoValue)

		// check delete,write,delete existing
		source = `byte "ALGO"
app_global_del
byte "ALGO"
int 0x78
app_global_put
byte "ALGO"
app_global_del
int 1
`
		delta = testApp(t, source, ep)
		require.Len(t, delta.GlobalDelta, 1)
		vd = delta.GlobalDelta["ALGO"]
		require.Equal(t, basics.DeleteAction, vd.Action)
		require.Empty(t, delta.LocalDeltas)

		ledger.Reset()
		ledger.Reset()
		ledger.NoGlobal(100, "ALGOA")
		ledger.NoGlobal(100, "ALGO")

		ledger.NewGlobal(100, "ALGO", algoValue)

		// check delete, write, delete non-existing
		source = `byte "ALGOA"   // key "ALGOA"
app_global_del
byte "ALGOA"
int 0x78
app_global_put
byte "ALGOA"
app_global_del
int 1
`
		delta = testApp(t, source, ep)
		require.Len(t, delta.GlobalDelta, 1)
		require.Len(t, delta.LocalDeltas, 0)
	})
}

func TestAppLocalDelete(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	testLogicRange(t, 2, 0, func(t *testing.T, ep *EvalParams, txn *transactions.Transaction, ledger *Ledger) {
		// check write/delete/read
		source := `int 0 // sender
byte "ALGO"
int 0x77              // value
app_local_put
int 1 // other
byte "ALGOA"     // key "ALGOA"
byte "ALGO"
app_local_put
int 0 // sender
byte "ALGO"
app_local_del
int 1 // other
byte "ALGOA"
app_local_del
int 0 // sender
int 0 // app
byte "ALGO"
app_local_get_ex
bnz error
int 1 // other
int 100
byte "ALGOA"
app_local_get_ex
bnz error
==
bnz ok
error:
err
ok:
int 1
`
		txn.ApplicationID = 100
		ledger.NewAccount(txn.Sender, 1)
		ledger.NewApp(txn.Sender, 100, basics.AppParams{})
		ledger.NewLocals(txn.Sender, 100)
		ledger.NewAccount(txn.Receiver, 1)
		ledger.NewLocals(txn.Receiver, 100)

		ep.Trace = &strings.Builder{}

		if ep.Proto.LogicSigVersion < sharedResourcesVersion {
			delta := testApp(t, source, ep)
			require.Equal(t, 0, len(delta.GlobalDelta))
			require.Equal(t, 2, len(delta.LocalDeltas))
			ledger.Reset()
		}

		if ep.Proto.LogicSigVersion >= directRefEnabledVersion {
			// test that app_local_put and _app_local_del can use byte addresses
			withBytes := strings.ReplaceAll(source, "int 0 // sender", "txn Sender")
			withBytes = strings.ReplaceAll(withBytes, "int 1 // other", "txn Accounts 1")
			delta := testApp(t, withBytes, ep)
			// But won't even compile in old teal
			testProg(t, withBytes, directRefEnabledVersion-1,
				Expect{4, "app_local_put arg 0 wanted..."}, Expect{11, "app_local_del arg 0 wanted..."})
			require.Equal(t, 0, len(delta.GlobalDelta))
			require.Equal(t, 2, len(delta.LocalDeltas))
			ledger.Reset()
		}

		ledger.NoLocal(txn.Sender, 100, "ALGOA")
		ledger.NoLocal(txn.Sender, 100, "ALGO")
		ledger.NoLocal(txn.Receiver, 100, "ALGOA")
		ledger.NoLocal(txn.Receiver, 100, "ALGO")

		algoValue := basics.TealValue{Type: basics.TealUintType, Uint: 0x77}
		ledger.NewLocal(txn.Sender, 100, "ALGO", algoValue)

		// check delete existing
		source = `txn Sender
byte "ALGO"
app_local_del
txn Sender
int 100
byte "ALGO"
app_local_get_ex
==  // two zeros
`

		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
		}
		delta := testApp(t, source, ep)
		require.Equal(t, 0, len(delta.GlobalDelta))
		require.Equal(t, 1, len(delta.LocalDeltas))
		vd := delta.LocalDeltas[0]["ALGO"]
		require.Equal(t, basics.DeleteAction, vd.Action)
		require.Equal(t, uint64(0), vd.Uint)
		require.Equal(t, "", vd.Bytes)

		ledger.Reset()
		ledger.NoLocal(txn.Sender, 100, "ALGOA")
		ledger.NoLocal(txn.Sender, 100, "ALGO")

		ledger.NewLocal(txn.Sender, 100, "ALGO", algoValue)

		// check delete and write non-existing
		source = `txn Sender
byte "ALGOA"
app_local_del
txn Sender
int 0
byte "ALGOA"
app_local_get_ex
==  // two zeros
txn Sender
byte "ALGOA"
int 0x78
app_local_put
`
		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
		}
		delta = testApp(t, source, ep)
		require.Equal(t, 0, len(delta.GlobalDelta))
		require.Equal(t, 1, len(delta.LocalDeltas))
		vd = delta.LocalDeltas[0]["ALGOA"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x78), vd.Uint)
		require.Equal(t, "", vd.Bytes)

		ledger.Reset()
		ledger.NoLocal(txn.Sender, 100, "ALGOA")
		ledger.NoLocal(txn.Sender, 100, "ALGO")

		ledger.NewLocal(txn.Sender, 100, "ALGO", algoValue)

		// check delete and write existing
		source = `txn Sender
byte "ALGO"
app_local_del
txn Sender
byte "ALGO"
int 0x78
app_local_put
int 1
`
		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
		}
		delta = testApp(t, source, ep)
		require.Equal(t, 0, len(delta.GlobalDelta))
		require.Equal(t, 1, len(delta.LocalDeltas))
		vd = delta.LocalDeltas[0]["ALGO"]
		require.Equal(t, basics.SetUintAction, vd.Action)
		require.Equal(t, uint64(0x78), vd.Uint)
		require.Equal(t, "", vd.Bytes)

		ledger.Reset()
		ledger.NoLocal(txn.Sender, 100, "ALGOA")
		ledger.NoLocal(txn.Sender, 100, "ALGO")

		ledger.NewLocal(txn.Sender, 100, "ALGO", algoValue)

		// check delete,write,delete existing
		source = `txn Sender
byte "ALGO"
app_local_del
txn Sender
byte "ALGO"
int 0x78
app_local_put
txn Sender
byte "ALGO"
app_local_del
int 1
`
		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
		}
		delta = testApp(t, source, ep)
		require.Equal(t, 0, len(delta.GlobalDelta))
		require.Equal(t, 1, len(delta.LocalDeltas))
		vd = delta.LocalDeltas[0]["ALGO"]
		require.Equal(t, basics.DeleteAction, vd.Action)
		require.Equal(t, uint64(0), vd.Uint)
		require.Equal(t, "", vd.Bytes)

		ledger.Reset()
		ledger.NoLocal(txn.Sender, 100, "ALGOA")
		ledger.NoLocal(txn.Sender, 100, "ALGO")

		ledger.NewLocal(txn.Sender, 100, "ALGO", algoValue)

		// check delete, write, delete non-existing
		source = `txn Sender
byte "ALGOA"
app_local_del
txn Sender
byte "ALGOA"
int 0x78
app_local_put
txn Sender
byte "ALGOA"
app_local_del
int 1
`
		if ep.Proto.LogicSigVersion < directRefEnabledVersion {
			source = strings.ReplaceAll(source, "txn Sender", "int 0")
		}
		delta = testApp(t, source, ep)
		require.Equal(t, 0, len(delta.GlobalDelta))
		require.Equal(t, 1, len(delta.LocalDeltas))
		require.Equal(t, 1, len(delta.LocalDeltas[0]))
	})
}

func TestEnumFieldErrors(t *testing.T) { // nolint:paralleltest // manipulates txnFieldSpecs
	partitiontest.PartitionTest(t)

	source := `txn Amount`
	origSpec := txnFieldSpecs[Amount]
	changed := origSpec
	changed.ftype = StackBytes
	txnFieldSpecs[Amount] = changed
	defer func() {
		txnFieldSpecs[Amount] = origSpec
	}()

	testLogic(t, source, AssemblerMaxVersion, defaultEvalParams(), "Amount expected field type is []byte but got uint64")
	testApp(t, source, defaultEvalParams(), "Amount expected field type is []byte but got uint64")

	source = `global MinTxnFee`

	origMinTxnFs := globalFieldSpecs[MinTxnFee]
	badMinTxnFs := origMinTxnFs
	badMinTxnFs.ftype = StackBytes
	globalFieldSpecs[MinTxnFee] = badMinTxnFs
	defer func() {
		globalFieldSpecs[MinTxnFee] = origMinTxnFs
	}()

	testLogic(t, source, AssemblerMaxVersion, defaultEvalParams(), "MinTxnFee expected field type is []byte but got uint64")
	testApp(t, source, defaultEvalParams(), "MinTxnFee expected field type is []byte but got uint64")

	ep, tx, ledger := makeSampleEnv()
	ledger.NewAccount(tx.Sender, 1)
	params := basics.AssetParams{
		Total:         1000,
		Decimals:      2,
		DefaultFrozen: false,
		UnitName:      "ALGO",
		AssetName:     "",
		URL:           string(protocol.PaymentTx),
		Manager:       tx.Sender,
		Reserve:       tx.Receiver,
		Freeze:        tx.Receiver,
		Clawback:      tx.Receiver,
	}
	ledger.NewAsset(tx.Sender, 55, params)

	source = `txn Sender
int 55
asset_holding_get AssetBalance
assert
`
	origBalanceFs := assetHoldingFieldSpecs[AssetBalance]
	badBalanceFs := origBalanceFs
	badBalanceFs.ftype = StackBytes
	assetHoldingFieldSpecs[AssetBalance] = badBalanceFs
	defer func() {
		assetHoldingFieldSpecs[AssetBalance] = origBalanceFs
	}()

	testApp(t, source, ep, "AssetBalance expected field type is []byte but got uint64")

	source = `int 55
asset_params_get AssetTotal
assert
`
	origTotalFs := assetParamsFieldSpecs[AssetTotal]
	badTotalFs := origTotalFs
	badTotalFs.ftype = StackBytes
	assetParamsFieldSpecs[AssetTotal] = badTotalFs
	defer func() {
		assetParamsFieldSpecs[AssetTotal] = origTotalFs
	}()

	testApp(t, source, ep, "AssetTotal expected field type is []byte but got uint64")
}

func TestReturnTypes(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	// Ensure all opcodes return values they are supposed to according to the OpSpecs table
	typeToArg := map[StackType]string{
		StackUint64: "int 1\n",
		StackAny:    "int 1\n",
		StackBytes:  "byte 0x33343536\n", // Which is the string "3456"
	}

	// We try to form a snippet that will test every opcode, by sandwiching it
	// between arguments that correspond to the opcode's input types, and then
	// check to see if the proper output types end up on the stack.  But many
	// opcodes require more specific inputs than a constant string or the number
	// 1 for ints.  Defaults are also supplied for immediate arguments.  For
	// opcodes that need to set up their own stack inputs, a ": at the front of
	// the string means "start with an empty stack".
	specialCmd := map[string]string{
		"txn":              "txn Sender",
		"txna":             "txna ApplicationArgs 0",
		"gtxn":             "gtxn 0 Sender",
		"gtxna":            "gtxna 0 ApplicationArgs 0",
		"global":           "global MinTxnFee",
		"gaids":            ": int 0; gaids",
		"gloads":           ": int 0; gloads 0",       // Needs txn index = 0 to work
		"gloadss":          ": int 0; int 1; gloadss", // Needs txn index = 0 to work
		"intc":             "intcblock 0; intc 0",
		"intc_0":           "intcblock 0; intc_0",
		"intc_1":           "intcblock 0 0; intc_1",
		"intc_2":           "intcblock 0 0 0; intc_2",
		"intc_3":           "intcblock 0 0 0 0; intc_3",
		"bytec":            "bytecblock 0x32; bytec 0",
		"bytec_0":          "bytecblock 0x32; bytec_0",
		"bytec_1":          "bytecblock 0x32 0x33; bytec_1",
		"bytec_2":          "bytecblock 0x32 0x33 0x34; bytec_2",
		"bytec_3":          "bytecblock 0x32 0x33 0x34 0x35; bytec_3",
		"substring":        "substring 0 2",
		"extract_uint32":   ": byte 0x0102030405; int 1; extract_uint32",
		"extract_uint64":   ": byte 0x010203040506070809; int 1; extract_uint64",
		"replace2":         ": byte 0x0102030405; byte 0x0809; replace2 2",
		"replace3":         ": byte 0x0102030405; int 2; byte 0x0809; replace3",
		"asset_params_get": "asset_params_get AssetUnitName",
		"gtxns":            "gtxns Sender",
		"gtxnsa":           ": int 0; gtxnsa ApplicationArgs 0",
		"app_params_get":   "app_params_get AppGlobalNumUint",
		"extract":          "extract 0 2",
		"txnas":            "txnas ApplicationArgs",
		"gtxnas":           "gtxnas 0 ApplicationArgs",
		"gtxnsas":          ": int 0; int 0; gtxnsas ApplicationArgs",
		"divw":             ": int 1; int 2; int 3; divw",

		// opcodes that require addresses, not just bytes
		"balance":         ": txn Sender; balance",
		"min_balance":     ": txn Sender; min_balance",
		"acct_params_get": ": txn Sender; acct_params_get AcctMinBalance",
		// Use "bury" here to take advantage of args pushed on stack by test
		"app_local_get":     "txn Accounts 1; bury 2; app_local_get",
		"app_local_get_ex":  "txn Accounts 1; bury 3; app_local_get_ex",
		"app_local_del":     "txn Accounts 1; bury 2; app_local_del",
		"app_local_put":     "txn Accounts 1; bury 3; app_local_put",
		"app_opted_in":      "txn Sender; bury 2; app_opted_in",
		"asset_holding_get": "txn Sender; bury 2; asset_holding_get AssetBalance",

		"itxn_field":  "itxn_begin; itxn_field TypeEnum",
		"itxn_next":   "itxn_begin; int pay; itxn_field TypeEnum; itxn_next",
		"itxn_submit": "itxn_begin; int pay; itxn_field TypeEnum; itxn_submit",
		"itxn":        "itxn_begin; int pay; itxn_field TypeEnum; itxn_submit; itxn CreatedAssetID",
		"itxna":       "itxn_begin; int pay; itxn_field TypeEnum; itxn_submit; itxna Accounts 0",
		"itxnas":      ": itxn_begin; int pay; itxn_field TypeEnum; itxn_submit; int 0; itxnas Accounts",
		"gitxn":       "itxn_begin; int pay; itxn_field TypeEnum; itxn_submit; gitxn 0 Sender",
		"gitxna":      "itxn_begin; int pay; itxn_field TypeEnum; itxn_submit; gitxna 0 Accounts 0",
		"gitxnas":     ": itxn_begin; int pay; itxn_field TypeEnum; itxn_submit; int 0; gitxnas 0 Accounts",

		"base64_decode": `: byte "YWJjMTIzIT8kKiYoKSctPUB+"; base64_decode StdEncoding`,
		"json_ref":      `: byte "{\"k\": 7}"; byte "k"; json_ref JSONUint64`,

		"block": "block BlkSeed",

		"proto": "callsub p; p: proto 0 3",
		"bury":  ": int 1; int 2; int 3; bury 2; pop; pop;",

		"box_create": "int 9; +; box_create",                 // make the size match the 10 in CreateBox
		"box_put":    "byte 0x010203040506; concat; box_put", // make the 4 byte arg into a 10
	}

	/* Make sure the specialCmd tests the opcode in question */
	for opcode, cmd := range specialCmd {
		assert.Contains(t, cmd, opcode)
	}

	// these have strange stack semantics or require special input data /
	// context, so they must be tested separately
	skipCmd := map[string]bool{
		"retsub": true,
		"err":    true,
		"return": true,

		"ed25519verify":       true,
		"ed25519verify_bare":  true,
		"ecdsa_verify":        true,
		"ecdsa_pk_recover":    true,
		"ecdsa_pk_decompress": true,

		"vrf_verify": true,

		"frame_dig":  true, // would need a "proto" subroutine
		"frame_bury": true, // would need a "proto" subroutine

		"bn256_add":        true,
		"bn256_scalar_mul": true,
		"bn256_pairing":    true,
	}

	byName := OpsByName[LogicVersion]
	for _, m := range []RunMode{ModeSig, ModeApp} {
		for name, spec := range byName {
			// Only try an opcode in its modes
			if (m & spec.Modes) == 0 {
				continue
			}
			if skipCmd[name] || spec.trusted {
				continue
			}
			m, name, spec := m, name, spec
			t.Run(fmt.Sprintf("mode=%s,opcode=%s", m, name), func(t *testing.T) {
				t.Parallel()

				provideStackInput := true
				cmd := name
				if special, ok := specialCmd[name]; ok {
					if strings.HasPrefix(special, ":") {
						cmd = special[1:]
						provideStackInput = false
					} else {
						cmd = special
					}
				} else {
					for _, imm := range spec.OpDetails.Immediates {
						switch imm.kind {
						case immByte:
							cmd += " 0"
						case immInt8:
							cmd += " -2"
						case immInt:
							cmd += " 10"
						case immInts:
							cmd += " 11 12 13"
						case immBytes:
							cmd += " 0x123456"
						case immBytess:
							cmd += " 0x12 0x34 0x56"
						case immLabel:
							cmd += " done; done: ;"
						case immLabels:
							cmd += " done1 done2; done1: ; done2: ;"
						default:
							require.Fail(t, "bad immediate", "%s", imm)
						}
					}
				}
				var sb strings.Builder
				if provideStackInput {
					for _, t := range spec.Arg.Types {
						sb.WriteString(typeToArg[t])
					}
				}
				sb.WriteString(cmd + "\n")
				ops := testProg(t, sb.String(), AssemblerMaxVersion)

				ep, tx, ledger := makeSampleEnv()

				tx.Type = protocol.ApplicationCallTx
				tx.ApplicationID = 1
				tx.ForeignApps = []basics.AppIndex{tx.ApplicationID}
				tx.ForeignAssets = []basics.AssetIndex{basics.AssetIndex(1), basics.AssetIndex(1)}
				tx.Boxes = []transactions.BoxRef{{
					Name: []byte("3456"),
				}}
				ep.TxnGroup[0].Lsig.Args = [][]byte{
					[]byte("aoeu"),
					[]byte("aoeu"),
					[]byte("aoeu2"),
					[]byte("aoeu3"),
				}
				// We are going to run with GroupIndex=1, so make tx1 interesting too (so
				// txn can look at things)
				ep.TxnGroup[1] = ep.TxnGroup[0]

				ep.pastScratch[0] = &scratchSpace{} // for gload
				ledger.NewAccount(tx.Sender, 1)
				params := basics.AssetParams{
					Total:         1000,
					Decimals:      2,
					DefaultFrozen: false,
					UnitName:      "ALGO",
					AssetName:     "",
					URL:           string(protocol.PaymentTx),
					Manager:       tx.Sender,
					Reserve:       tx.Receiver,
					Freeze:        tx.Receiver,
					Clawback:      tx.Receiver,
				}
				ledger.NewAsset(tx.Sender, 1, params)
				ledger.NewApp(tx.Sender, 1, basics.AppParams{})
				ledger.NewAccount(tx.Receiver, 1000000)
				ledger.NewLocals(tx.Receiver, 1)
				key, err := hex.DecodeString("33343536")
				require.NoError(t, err)
				algoValue := basics.TealValue{Type: basics.TealUintType, Uint: 0x77}
				ledger.NewLocal(tx.Receiver, 1, string(key), algoValue)
				ledger.NewAccount(appAddr(1), 1000000)

				ep.reset()                          // for Trace and budget isolation
				ep.pastScratch[0] = &scratchSpace{} // for gload
				// these allows the box_* opcodes that to work
				ledger.CreateBox(1, "3456", 10)
				ep.ioBudget = 50

				cx := EvalContext{
					EvalParams:   ep,
					runModeFlags: m,
					groupIndex:   1,
					txn:          &ep.TxnGroup[1],
					appID:        1,
				}

				// These set conditions for some ops that examine the group.
				// This convinces them all to work.  Revisit.
				cx.TxnGroup[0].ConfigAsset = 100

				// These little programs need not pass. Since the returned stack
				// is checked for typing, we can't get hung up on whether it is
				// exactly one positive int. But if it fails for any *other*
				// reason, we're not doing a good test.
				_, err = eval(ops.Program, &cx)
				if err != nil {
					// Allow the kinds of errors we expect, but fail for stuff
					// that indicates the opcode itself failed.
					reason := err.Error()
					if reason != "stack finished with bytes not int" &&
						!strings.HasPrefix(reason, "stack len is") {
						require.NoError(t, err, "%s: %s\n%s", name, err, ep.Trace)
					}
				}
				require.Len(t, cx.stack, len(spec.Return.Types), "%s", ep.Trace)
				for i := 0; i < len(spec.Return.Types); i++ {
					stackType := cx.stack[i].argType()
					retType := spec.Return.Types[i]
					require.True(
						t, typecheck(retType, stackType),
						"%s expected to return %s but actual is %s", spec.Name, retType, stackType,
					)
				}
			})
		}
	}
}

func TestTxnEffects(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()
	ep, _, _ := makeSampleEnv()
	// We don't allow the effects fields to see the current or future transactions
	testApp(t, "byte 0x32; log; txn NumLogs; int 1; ==", ep, "txn effects can only be read from past txns")
	testApp(t, "byte 0x32; log; txn Logs 0; byte 0x32; ==", ep, "txn effects can only be read from past txns")
	testApp(t, "byte 0x32; log; txn LastLog; byte 0x32; ==", ep, "txn effects can only be read from past txns")
	testApp(t, "byte 0x32; log; gtxn 0 NumLogs; int 1; ==", ep, "txn effects can only be read from past txns")
	testApp(t, "byte 0x32; log; gtxn 0 Logs 0; byte 0x32; ==", ep, "txn effects can only be read from past txns")
	testApp(t, "byte 0x32; log; gtxn 0 LastLog; byte 0x32; ==", ep, "txn effects can only be read from past txns")

	// Look at the logs of tx 0
	testApps(t, []string{"", "byte 0x32; log; gtxn 0 LastLog; byte 0x; =="}, nil, AssemblerMaxVersion, nil)
	testApps(t, []string{"byte 0x33; log; int 1", "gtxn 0 LastLog; byte 0x33; =="}, nil, AssemblerMaxVersion, nil)
	testApps(t, []string{"byte 0x33; dup; log; log; int 1", "gtxn 0 NumLogs; int 2; =="}, nil, AssemblerMaxVersion, nil)
	testApps(t, []string{"byte 0x37; log; int 1", "gtxn 0 Logs 0; byte 0x37; =="}, nil, AssemblerMaxVersion, nil)
	testApps(t, []string{"byte 0x37; log; int 1", "int 0; gtxnas 0 Logs; byte 0x37; =="}, nil, AssemblerMaxVersion, nil)

	// Look past the logs of tx 0
	testApps(t, []string{"byte 0x37; log; int 1", "gtxna 0 Logs 1; byte 0x37; =="}, nil, AssemblerMaxVersion, nil,
		Expect{1, "invalid Logs index 1"})
	testApps(t, []string{"byte 0x37; log; int 1", "int 6; gtxnas 0 Logs; byte 0x37; =="}, nil, AssemblerMaxVersion, nil,
		Expect{1, "invalid Logs index 6"})
}

func TestRound(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()
	ep, _, _ := makeSampleEnv()
	source := "global Round; int 1; >="
	testApp(t, source, ep)
}

func TestLatestTimestamp(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()
	ep, _, _ := makeSampleEnv()
	source := "global LatestTimestamp; int 1; >="
	testApp(t, source, ep)
}

func TestBlockSeed(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	ep, txn, l := makeSampleEnv()

	// makeSampleEnv creates txns with fv, lv that don't actually fit the round
	// in l.  Nothing in most tests cares. But the rule for `block` is related
	// to lv and fv, so we set the fv,lv more realistically.
	txn.FirstValid = l.Round() - 10
	txn.LastValid = l.Round() + 10

	// Keep in mind that proto.MaxTxnLife is 1500 in the test proto

	// l.round() is 0xffffffff+5 = 4294967300 in test ledger

	// These first two tests show that current-1 is not available now, though a
	// resonable extension is to allow such access for apps (not sigs).
	testApp(t, "int 4294967299; block BlkSeed; len; int 32; ==", ep,
		"not available") // current - 1
	testApp(t, "int 4294967300; block BlkSeed; len; int 32; ==", ep,
		"not available") // can't get current round's blockseed

	testApp(t, "int 4294967300; int 1500; -; block BlkSeed; len; int 32; ==", ep,
		"not available") // 1500 back from current is more than 1500 back from lv
	testApp(t, "int 4294967310; int 1500; -; block BlkSeed; len; int 32; ==", ep) // 1500 back from lv is legal
	testApp(t, "int 4294967310; int 1501; -; block BlkSeed; len; int 32; ==", ep) // 1501 back from lv is legal
	testApp(t, "int 4294967310; int 1502; -; block BlkSeed; len; int 32; ==", ep,
		"not available") // 1501 back from lv is not

	// A little silly, as it only tests the test ledger: ensure samenes and differentness
	testApp(t, "int 0xfffffff0; block BlkSeed; int 0xfffffff0; block BlkSeed; ==", ep)
	testApp(t, "int 0xfffffff0; block BlkSeed; int 0xfffffff1; block BlkSeed; !=", ep)

	// `block` should also work in LogicSigs, to drive home the point, blot out
	// the normal Ledger
	ep.Ledger = nil
	testLogic(t, "int 0xfffffff0; block BlkTimestamp", randomnessVersion, ep)
}

func TestCurrentApplicationID(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()
	ep, tx, _ := makeSampleEnv()
	tx.ApplicationID = 42
	source := "global CurrentApplicationID; int 42; =="
	testApp(t, source, ep)
}

func TestAppLoop(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()
	ep, _, _ := makeSampleEnv()

	stateful := "global CurrentApplicationID; pop;"

	// Double until > 10. Should be 16
	testApp(t, stateful+"int 1; loop: int 2; *; dup; int 10; <; bnz loop; int 16; ==", ep)

	// Infinite loop because multiply by one instead of two
	testApp(t, stateful+"int 1; loop:; int 1; *; dup; int 10; <; bnz loop; int 16; ==", ep, "dynamic cost")
}

func TestPooledAppCallsVerifyOp(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	source := `
	global CurrentApplicationID
	pop
	byte 0x01
	byte "ZC9KNzlnWTlKZ1pwSkNzQXVzYjNBcG1xTU9YbkRNWUtIQXNKYVk2RzRBdExPakQx"
	addr DROUIZXGT3WFJR3QYVZWTR5OJJXJCMOLS7G4FUGZDSJM5PNOVOREH6HIZE
	ed25519verify
	pop
	int 1`

	ledger := NewLedger(nil)
	call := transactions.SignedTxn{Txn: transactions.Transaction{Type: protocol.ApplicationCallTx}}
	// Simulate test with 2 grouped txn
	testApps(t, []string{source, ""}, []transactions.SignedTxn{call, call}, LogicVersion, ledger,
		Expect{0, "pc=107 dynamic cost budget exceeded, executing ed25519verify: local program cost was 5"})

	// Simulate test with 3 grouped txn
	testApps(t, []string{source, "", ""}, []transactions.SignedTxn{call, call, call}, LogicVersion, ledger)
}

func appAddr(id int) basics.Address {
	return basics.AppIndex(id).Address()
}

func TestAppInfo(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	ep, tx, ledger := makeSampleEnv()
	require.Equal(t, 888, int(tx.ApplicationID))
	ledger.NewApp(tx.Receiver, 888, basics.AppParams{})
	testApp(t, "global CurrentApplicationID; int 888; ==;", ep)
	source := fmt.Sprintf("global CurrentApplicationAddress; addr %s; ==;", appAddr(888))
	testApp(t, source, ep)

	source = fmt.Sprintf("int 0; app_params_get AppAddress; assert; addr %s; ==;", appAddr(888))
	testApp(t, source, ep)

	// To document easy construction:
	// python -c 'import algosdk.encoding as e; print(e.encode_address(e.checksum(b"appID"+(888).to_bytes(8, "big"))))'
	a := "U7C5FUHZM5PL5EIS2KHHLL456GS66DZBEEKL2UBQLMKH2X5X5I643ZIM6U"
	source = fmt.Sprintf("int 0; app_params_get AppAddress; assert; addr %s; ==;", a)
	testApp(t, source, ep)
}

func TestBudget(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	source := `
global OpcodeBudget
int 699
==
assert
global OpcodeBudget
int 695
==
`
	testApp(t, source, defaultEvalParams())
}

func TestSelfMutateV8(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	ep, _, ledger := makeSampleEnvWithVersion(8)

	/* In order to test the added protection of mutableAccountReference, we're
	   going to set up a ledger in which an app account is opted into
	   itself. That was impossible before v6, and indeed we did not have the
	   extra mutable reference check then. */
	ledger.NewLocals(basics.AppIndex(888).Address(), 888)
	ledger.NewLocal(basics.AppIndex(888).Address(), 888, "hey",
		basics.TealValue{Type: basics.TealUintType, Uint: 77})

	source := `
global CurrentApplicationAddress
byte "hey"
int 42
app_local_put
`
	testApp(t, source, ep, "invalid Account reference for mutation")

	source = `
global CurrentApplicationAddress
byte "hey"
app_local_del
`
	testApp(t, source, ep, "invalid Account reference for mutation")

	/* But let's just check read access is working properly. */
	source = `
global CurrentApplicationAddress
byte "hey"
app_local_get
int 77
==
`
	testApp(t, source, ep)
}

// TestSelfMutateV9AndUp tests that apps can mutate their own app's local state
// starting with v9. Includes tests to the EvalDelta created.
func TestSelfMutateV9AndUp(t *testing.T) {
	partitiontest.PartitionTest(t)
	t.Parallel()

	// start at 9, when such mutation became legal
	testLogicRange(t, 9, 0, func(t *testing.T, ep *EvalParams, tx *transactions.Transaction, ledger *Ledger) {
		/* In order to test that apps can now mutate their own app's local state,
		   we're going to set up a ledger in which an app account is opted into
		   itself. */
		ledger.NewLocals(basics.AppIndex(888).Address(), 888)
		ledger.NewLocal(basics.AppIndex(888).Address(), 888, "hey",
			basics.TealValue{Type: basics.TealUintType, Uint: 77})

		// and we'll modify the passed account's locals, to better check the ED
		ledger.NewLocals(tx.Accounts[0], 888)

		source := `
global CurrentApplicationAddress
byte "hey"
int 42
app_local_put
txn Accounts 1
byte "acct"
int 43
app_local_put
int 1
`
		ed := testApp(t, source, ep)
		require.Len(t, tx.Accounts, 1) // Sender + 1 tx.Accounts means LocalDelta index should be 2
		require.Equal(t, map[uint64]basics.StateDelta{
			1: {
				"acct": {
					Action: basics.SetUintAction,
					Uint:   43,
				},
			},
			2: {
				"hey": {
					Action: basics.SetUintAction,
					Uint:   42,
				},
			},
		}, ed.LocalDeltas)
		require.Equal(t, []basics.Address{tx.ApplicationID.Address()}, ed.SharedAccts)

		/* Confirm it worked. */
		source = `
global CurrentApplicationAddress
byte "hey"
app_local_get
int 42
==
`
		testApp(t, source, ep)

		source = `
global CurrentApplicationAddress
byte "hey"
int 10
app_local_put					// this will get wiped out by del
global CurrentApplicationAddress
byte "hey"
app_local_del
txn Accounts 1
byte "acct"
int 7
app_local_put
int 1
`
		ed = testApp(t, source, ep)
		require.Len(t, tx.Accounts, 1) // Sender + 1 tx.Accounts means LocalDelta index should be 2
		require.Equal(t, map[uint64]basics.StateDelta{
			1: {
				"acct": {
					Action: basics.SetUintAction,
					Uint:   7,
				},
			},
			2: {
				"hey": {
					Action: basics.DeleteAction,
				},
			},
		}, ed.LocalDeltas)
		require.Equal(t, []basics.Address{tx.ApplicationID.Address()}, ed.SharedAccts)

		// Now, repeat the "put" test with multiple keys, to ensure only one
		// address is added to SharedAccts and we'll modify the Sender too, to
		// better check the ED
		ledger.NewLocals(tx.Sender, 888)

		source = `
txn Sender
byte "hey"
int 40
app_local_put

global CurrentApplicationAddress
byte "hey"
int 42
app_local_put

global CurrentApplicationAddress
byte "joe"
int 21
app_local_put
int 1
`
		ed = testApp(t, source, ep)
		require.Len(t, tx.Accounts, 1) // Sender + 1 tx.Accounts means LocalDelta index should be 2
		require.Equal(t, map[uint64]basics.StateDelta{
			0: {
				"hey": {
					Action: basics.SetUintAction,
					Uint:   40,
				},
			},
			2: {
				"hey": {
					Action: basics.SetUintAction,
					Uint:   42,
				},
				"joe": {
					Action: basics.SetUintAction,
					Uint:   21,
				},
			},
		}, ed.LocalDeltas)

		require.Equal(t, []basics.Address{tx.ApplicationID.Address()}, ed.SharedAccts)
	})
}
