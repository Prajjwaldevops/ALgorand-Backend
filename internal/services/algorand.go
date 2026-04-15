package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"

	"bountyvault/internal/config"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/client/v2/indexer"
	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/v2/mnemonic"
	"github.com/algorand/go-algorand-sdk/v2/transaction"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

// AlgorandService handles blockchain interactions
type AlgorandService struct {
	algodClient   *algod.Client
	indexerClient *indexer.Client
	appID         uint64
	network       string
}

// NewAlgorandService creates a new Algorand service instance
func NewAlgorandService(cfg *config.Config) (*AlgorandService, error) {
	algodClient, err := algod.MakeClient(cfg.AlgoNodeURL, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create algod client: %w", err)
	}

	indexerClient, err := indexer.MakeClient(cfg.AlgoIndexerURL, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create indexer client: %w", err)
	}

	return &AlgorandService{
		algodClient:   algodClient,
		indexerClient: indexerClient,
		appID:         cfg.BountyAppID,
		network:       cfg.AlgoNetwork,
	}, nil
}

// GetSuggestedParams fetches current network parameters for transactions
func (s *AlgorandService) GetSuggestedParams(ctx context.Context) (types.SuggestedParams, error) {
	params, err := s.algodClient.SuggestedParams().Do(ctx)
	if err != nil {
		return types.SuggestedParams{}, fmt.Errorf("failed to get suggested params: %w", err)
	}
	return params, nil
}

// UnsignedTxnResult holds encoded unsigned transactions for client-side signing
type UnsignedTxnResult struct {
	Transactions []string `json:"transactions"` // base64 encoded unsigned transactions
	GroupID      string   `json:"group_id"`      // base64 encoded group ID
	Message      string   `json:"message"`
}

// BuildCreateBountyTxns builds unsigned transactions for creating a bounty
// Returns base64-encoded transactions for the frontend to sign with wallet
func (s *AlgorandService) BuildCreateBountyTxns(
	ctx context.Context,
	creatorAddr string,
	rewardMicroAlgos uint64,
	termsHash []byte,
	deadline uint64,
	maxSubmissions uint64,
	arbitratorAddr string,
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	appAddr := crypto.GetApplicationAddress(s.appID)

	// Transaction 1: Payment to escrow
	payTxn, err := transaction.MakePaymentTxn(
		creatorAddr,
		appAddr.String(),
		rewardMicroAlgos,
		nil,
		"",
		params,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment txn: %w", err)
	}

	// ABI method selector for create_bounty(pay,uint64,uint64)string
	methodSelector := []byte{0xf9, 0x3b, 0xe3, 0xfd}
	// NOTE: termsHash is NOT part of the on-chain ABI; it is stored off-chain only.
	// The create_bounty method only expects: grouped pay txn + deadline + max_submissions.
	appArgs := [][]byte{
		methodSelector,
		uint64ToBytes(deadline),
		uint64ToBytes(maxSubmissions),
	}

	var foreignAccounts []string
	if arbitratorAddr != "" {
		foreignAccounts = []string{arbitratorAddr}
	}

	senderAddr, err := types.DecodeAddress(creatorAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid creator address: %w", err)
	}

	appCallTxn, err := transaction.MakeApplicationNoOpTx(
		s.appID,
		appArgs,
		foreignAccounts,
		nil, nil,
		params,
		senderAddr, nil, types.Digest{}, [32]byte{}, types.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create app call txn: %w", err)
	}

	// Group transactions
	gid, err := crypto.ComputeGroupID([]types.Transaction{payTxn, appCallTxn})
	if err != nil {
		return nil, fmt.Errorf("failed to compute group ID: %w", err)
	}
	payTxn.Group = gid
	appCallTxn.Group = gid

	// Encode transactions
	payTxnBytes := encodeMsgpack(payTxn)
	appCallTxnBytes := encodeMsgpack(appCallTxn)

	return &UnsignedTxnResult{
		Transactions: []string{
			base64.StdEncoding.EncodeToString(payTxnBytes),
			base64.StdEncoding.EncodeToString(appCallTxnBytes),
		},
		GroupID: base64.StdEncoding.EncodeToString(gid[:]),
		Message: "Sign both transactions with your wallet",
	}, nil
}

// BuildSubmitProofTxn builds an unsigned transaction for submitting work proof
func (s *AlgorandService) BuildSubmitProofTxn(
	ctx context.Context,
	workerAddr string,
	workHash []byte,
	megaHash []byte,
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	methodSelector := []byte{0x7b, 0xf5, 0x65, 0xf2} // submit_proof(byte[],byte[])string
	appArgs := [][]byte{
		methodSelector,
		workHash,
		megaHash,
	}

	senderAddr, err := types.DecodeAddress(workerAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid worker address: %w", err)
	}

	appCallTxn, err := transaction.MakeApplicationNoOpTx(
		s.appID,
		appArgs,
		nil, nil, nil,
		params,
		senderAddr, nil, types.Digest{}, [32]byte{}, types.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create submit proof txn: %w", err)
	}

	txnBytes := encodeMsgpack(appCallTxn)

	return &UnsignedTxnResult{
		Transactions: []string{
			base64.StdEncoding.EncodeToString(txnBytes),
		},
		Message: "Sign this transaction to submit your work proof",
	}, nil
}

// BuildApprovePayoutTxn builds an unsigned transaction for approving a worker's payout
func (s *AlgorandService) BuildApprovePayoutTxn(
	ctx context.Context,
	creatorAddr string,
	workerAddr string,
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	// approve_payout(address)string — worker address is an ABI arg, not just a foreign account
	methodSelector := []byte{0xa1, 0x19, 0x54, 0xa7}
	// ABI-encode the worker address as a 32-byte arg
	workerAddrDecoded, err := types.DecodeAddress(workerAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid worker address: %w", err)
	}

	appArgs := [][]byte{
		methodSelector,
		workerAddrDecoded[:],
	}
	// Worker must also be in the foreign accounts array for inner payment
	foreignAccounts := []string{workerAddr}

	senderAddr, err := types.DecodeAddress(creatorAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid creator address: %w", err)
	}

	// Set extra fee (2x min) to cover the inner payment txn via fee pooling
	params.Fee = 2000
	params.FlatFee = true

	appCallTxn, err := transaction.MakeApplicationNoOpTx(
		s.appID,
		appArgs,
		foreignAccounts,
		nil, nil,
		params,
		senderAddr, nil, types.Digest{}, [32]byte{}, types.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create approve payout txn: %w", err)
	}

	txnBytes := encodeMsgpack(appCallTxn)

	return &UnsignedTxnResult{
		Transactions: []string{
			base64.StdEncoding.EncodeToString(txnBytes),
		},
		Message: "Sign this transaction to approve the worker's payout",
	}, nil
}

// BuildDisputeTxn builds an unsigned transaction for initiating a dispute
func (s *AlgorandService) BuildDisputeTxn(
	ctx context.Context,
	senderAddr string,
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	// initiate_dispute()string — no extra args
	methodSelector := []byte{0x9a, 0xfe, 0x9a, 0xca}
	appArgs := [][]byte{
		methodSelector,
	}

	senderAddress, err := types.DecodeAddress(senderAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	appCallTxn, err := transaction.MakeApplicationNoOpTx(
		s.appID,
		appArgs,
		nil, nil, nil,
		params,
		senderAddress, nil, types.Digest{}, [32]byte{}, types.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create dispute txn: %w", err)
	}

	txnBytes := encodeMsgpack(appCallTxn)

	return &UnsignedTxnResult{
		Transactions: []string{
			base64.StdEncoding.EncodeToString(txnBytes),
		},
		Message: "Sign this transaction to initiate a dispute",
	}, nil
}

// BuildDAOVoteTxn builds an unsigned transaction for DAO voting
func (s *AlgorandService) BuildDAOVoteTxn(
	ctx context.Context,
	voterAddr string,
	support uint64, // 1=approve, 2=reject
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	methodSelector := []byte{0x69, 0x44, 0xa9, 0xde} // cast_dao_vote(uint64)string
	appArgs := [][]byte{
		methodSelector,
		uint64ToBytes(support),
	}

	senderAddr, err := types.DecodeAddress(voterAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid voter address: %w", err)
	}

	appCallTxn, err := transaction.MakeApplicationNoOpTx(
		s.appID,
		appArgs,
		nil, nil, nil,
		params,
		senderAddr, nil, types.Digest{}, [32]byte{}, types.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create DAO vote txn: %w", err)
	}

	txnBytes := encodeMsgpack(appCallTxn)

	return &UnsignedTxnResult{
		Transactions: []string{
			base64.StdEncoding.EncodeToString(txnBytes),
		},
		Message: "Sign this transaction to cast your DAO vote",
	}, nil
}

// BuildRejectSubmissionTxn builds an unsigned transaction for rejecting a worker's submission
func (s *AlgorandService) BuildRejectSubmissionTxn(
	ctx context.Context,
	creatorAddr string,
	workerAddr string,
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	// reject_submission(address)string
	methodSelector := []byte{0x5a, 0xe7, 0x67, 0xc8}
	workerAddrDecoded, err := types.DecodeAddress(workerAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid worker address: %w", err)
	}
	appArgs := [][]byte{methodSelector, workerAddrDecoded[:]}
	foreignAccounts := []string{workerAddr}

	senderAddr, err := types.DecodeAddress(creatorAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid creator address: %w", err)
	}

	appCallTxn, err := transaction.MakeApplicationNoOpTx(
		s.appID, appArgs, foreignAccounts, nil, nil,
		params, senderAddr, nil, types.Digest{}, [32]byte{}, types.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create reject submission txn: %w", err)
	}

	return &UnsignedTxnResult{
		Transactions: []string{base64.StdEncoding.EncodeToString(encodeMsgpack(appCallTxn))},
		Message:      "Sign this transaction to reject the submission",
	}, nil
}

// BuildCancelBountyTxn builds an unsigned transaction for cancelling a bounty
func (s *AlgorandService) BuildCancelBountyTxn(
	ctx context.Context,
	creatorAddr string,
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	// cancel_bounty()string — triggers inner payment (refund)
	methodSelector := []byte{0xe7, 0x98, 0x2d, 0xaa}
	appArgs := [][]byte{methodSelector}

	senderAddr, err := types.DecodeAddress(creatorAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid creator address: %w", err)
	}

	params.Fee = 2000
	params.FlatFee = true

	appCallTxn, err := transaction.MakeApplicationNoOpTx(
		s.appID, appArgs, nil, nil, nil,
		params, senderAddr, nil, types.Digest{}, [32]byte{}, types.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create cancel bounty txn: %w", err)
	}

	return &UnsignedTxnResult{
		Transactions: []string{base64.StdEncoding.EncodeToString(encodeMsgpack(appCallTxn))},
		Message:      "Sign this transaction to cancel the bounty and refund",
	}, nil
}

// BuildLetGoBountyTxn builds an unsigned transaction for a freelancer to let go of a bounty
func (s *AlgorandService) BuildLetGoBountyTxn(
	ctx context.Context,
	freelancerAddr string,
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	// let_go_bounty()string — triggers inner payment (refund to creator)
	methodSelector := []byte{0x57, 0xd6, 0x4b, 0x17}
	appArgs := [][]byte{methodSelector}

	senderAddr, err := types.DecodeAddress(freelancerAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid freelancer address: %w", err)
	}

	params.Fee = 2000
	params.FlatFee = true

	appCallTxn, err := transaction.MakeApplicationNoOpTx(
		s.appID, appArgs, nil, nil, nil,
		params, senderAddr, nil, types.Digest{}, [32]byte{}, types.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create let go bounty txn: %w", err)
	}

	return &UnsignedTxnResult{
		Transactions: []string{base64.StdEncoding.EncodeToString(encodeMsgpack(appCallTxn))},
		Message:      "Sign this transaction to release the bounty",
	}, nil
}

// BuildResolveDAODisputeTxn builds an unsigned transaction for resolving a DAO dispute
func (s *AlgorandService) BuildResolveDAODisputeTxn(
	ctx context.Context,
	senderAddr string,
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	// resolve_dao_dispute()string — triggers inner payment
	methodSelector := []byte{0xef, 0x86, 0x29, 0xb1}
	appArgs := [][]byte{methodSelector}

	sender, err := types.DecodeAddress(senderAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	params.Fee = 2000
	params.FlatFee = true

	appCallTxn, err := transaction.MakeApplicationNoOpTx(
		s.appID, appArgs, nil, nil, nil,
		params, sender, nil, types.Digest{}, [32]byte{}, types.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resolve dispute txn: %w", err)
	}

	return &UnsignedTxnResult{
		Transactions: []string{base64.StdEncoding.EncodeToString(encodeMsgpack(appCallTxn))},
		Message:      "Sign this transaction to resolve the DAO dispute",
	}, nil
}

// BuildRefundExpiredTxn builds an unsigned transaction for refunding an expired bounty
func (s *AlgorandService) BuildRefundExpiredTxn(
	ctx context.Context,
	senderAddr string,
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	// refund_expired()string — triggers inner payment
	methodSelector := []byte{0xfb, 0x2f, 0xe8, 0xa3}
	appArgs := [][]byte{methodSelector}

	sender, err := types.DecodeAddress(senderAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	params.Fee = 2000
	params.FlatFee = true

	appCallTxn, err := transaction.MakeApplicationNoOpTx(
		s.appID, appArgs, nil, nil, nil,
		params, sender, nil, types.Digest{}, [32]byte{}, types.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create refund expired txn: %w", err)
	}

	return &UnsignedTxnResult{
		Transactions: []string{base64.StdEncoding.EncodeToString(encodeMsgpack(appCallTxn))},
		Message:      "Sign this transaction to refund the expired bounty",
	}, nil
}

// BuildVotePaymentTxn builds an unsigned payment transaction for DAO voting:
//   Single Payment: voter → escrow address (0.001 ALGO = 1000 microAlgos gas fee)
//
// ESCROW BYPASS MODE: We skip the cast_dao_vote() app call because disputes
// are managed off-chain (database). The on-chain bounty status is never set to
// DISPUTED, so the smart contract assertion (status == 3) always fails.
// Once per-bounty contracts with on-chain disputes are enabled, re-add the app call.
func (s *AlgorandService) BuildVotePaymentTxn(
	ctx context.Context,
	voterAddr string,
	escrowAddr string,
	voteFor uint64, // 1=creator, 2=freelancer (recorded in DB)
	appID uint64,   // reserved for future per-bounty contract use
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	// Single payment: voter → escrow, 0.001 ALGO gas fee
	amountMicroAlgos := uint64(1000) // 0.001 ALGO
	note := []byte("BountyVault:dao_vote_gas_fee")

	payTxn, err := transaction.MakePaymentTxn(
		voterAddr,
		escrowAddr,
		amountMicroAlgos,
		note,
		"",
		params,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create vote payment txn: %w", err)
	}

	txnBytes := encodeMsgpack(payTxn)

	return &UnsignedTxnResult{
		Transactions: []string{
			base64.StdEncoding.EncodeToString(txnBytes),
		},
		Message: "Sign this transaction to cast your DAO vote (0.001 ALGO gas fee to escrow)",
	}, nil
}

// BuildFinalizeDisputeTxn builds an unsigned transaction to call resolve_dao_dispute()
// on the smart contract. This is permissionless — anyone can call after dao_deadline.
// The contract itself handles the inner payment to the winner.
func (s *AlgorandService) BuildFinalizeDisputeTxn(
	ctx context.Context,
	senderAddr string,
	appID uint64, // per-bounty contract app ID (0 = use default)
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	targetAppID := appID
	if targetAppID == 0 {
		targetAppID = s.appID
	}

	// ABI method selector: resolve_dao_dispute()string
	methodSelector := []byte{0xef, 0x86, 0x29, 0xb1}
	appArgs := [][]byte{methodSelector}

	sender, err := types.DecodeAddress(senderAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	// Extra fee for inner payment (contract pays freelancer/creator)
	params.Fee = 2000
	params.FlatFee = true

	appCallTxn, err := transaction.MakeApplicationNoOpTx(
		targetAppID, appArgs, nil, nil, nil,
		params, sender, nil, types.Digest{}, [32]byte{}, types.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resolve dispute txn: %w", err)
	}

	return &UnsignedTxnResult{
		Transactions: []string{base64.StdEncoding.EncodeToString(encodeMsgpack(appCallTxn))},
		Message:      "Sign this transaction to finalize the DAO dispute (contract releases funds to winner)",
	}, nil
}


// BuildEscrowLockTxn builds a simple payment transaction to the escrow address.
// Used when escrow bypass mode is active — funds go directly to the escrow account
// instead of the smart contract application address.
func (s *AlgorandService) BuildEscrowLockTxn(
	ctx context.Context,
	creatorAddr string,
	escrowAddr string,
	rewardMicroAlgos uint64,
	bountyID string,
) (*UnsignedTxnResult, error) {
	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return nil, err
	}

	note := []byte(fmt.Sprintf("BountyVault:escrow_lock:%s", bountyID))

	payTxn, err := transaction.MakePaymentTxn(
		creatorAddr,
		escrowAddr,
		rewardMicroAlgos,
		note,
		"",
		params,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create escrow lock payment txn: %w", err)
	}

	txnBytes := encodeMsgpack(payTxn)

	return &UnsignedTxnResult{
		Transactions: []string{
			base64.StdEncoding.EncodeToString(txnBytes),
		},
		Message: "Sign this transaction to lock funds in escrow",
	}, nil
}

// SendDirectPayment sends ALGO directly from the escrow account to a recipient.
// This bypasses the smart contract — used as a temporary workaround for payouts.
func (s *AlgorandService) SendDirectPayment(
	ctx context.Context,
	escrowMnemonic string,
	recipientAddr string,
	amountMicroAlgos uint64,
	note string,
) (string, error) {
	// Recover private key from mnemonic
	privateKey, err := mnemonic.ToPrivateKey(escrowMnemonic)
	if err != nil {
		return "", fmt.Errorf("failed to recover escrow key: %w", err)
	}

	// Derive sender address from private key
	account, err := crypto.AccountFromPrivateKey(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to get account from private key: %w", err)
	}
	senderAddr := account.Address.String()

	params, err := s.GetSuggestedParams(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get params: %w", err)
	}

	txn, err := transaction.MakePaymentTxn(
		senderAddr,
		recipientAddr,
		amountMicroAlgos,
		[]byte(note),
		"",
		params,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create payment txn: %w", err)
	}

	// Sign the transaction
	_, signedTxnBytes, err := crypto.SignTransaction(privateKey, txn)
	if err != nil {
		return "", fmt.Errorf("failed to sign txn: %w", err)
	}

	// Submit
	txID, err := s.algodClient.SendRawTransaction(signedTxnBytes).Do(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to submit payment: %w", err)
	}

	log.Printf("[ESCROW-BYPASS] Direct payment sent: %s -> %s (%d microAlgos) txID=%s", senderAddr, recipientAddr, amountMicroAlgos, txID)
	return txID, nil
}

// SubmitSignedTxns submits signed transactions to the Algorand network
func (s *AlgorandService) SubmitSignedTxns(ctx context.Context, signedTxnsB64 []string) (string, error) {
	var allTxnBytes []byte
	for _, b64 := range signedTxnsB64 {
		txnBytes, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return "", fmt.Errorf("failed to decode signed txn: %w", err)
		}
		allTxnBytes = append(allTxnBytes, txnBytes...)
	}

	txID, err := s.algodClient.SendRawTransaction(allTxnBytes).Do(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return txID, nil
}

// WaitForConfirmation waits for a transaction to be confirmed
func (s *AlgorandService) WaitForConfirmation(ctx context.Context, txID string, rounds uint64) error {
	status, err := s.algodClient.Status().Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	lastRound := status.LastRound
	for i := uint64(0); i < rounds; i++ {
		info, _, err := s.algodClient.PendingTransactionInformation(txID).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to get pending txn info: %w", err)
		}

		if info.ConfirmedRound > 0 {
			log.Printf("Transaction %s confirmed in round %d", txID, info.ConfirmedRound)
			return nil
		}

		// Wait for next round
		s.algodClient.StatusAfterBlock(lastRound + 1).Do(ctx)
		lastRound++
	}

	return fmt.Errorf("transaction %s not confirmed after %d rounds", txID, rounds)
}

// GetApplicationState reads the global state of the bounty escrow contract
func (s *AlgorandService) GetApplicationState(ctx context.Context) (map[string]interface{}, error) {
	app, err := s.algodClient.GetApplicationByID(s.appID).Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get application: %w", err)
	}

	state := make(map[string]interface{})
	for _, kv := range app.Params.GlobalState {
		keyBytes, _ := base64.StdEncoding.DecodeString(kv.Key)
		key := string(keyBytes)

		if kv.Value.Type == 1 { // bytes
			state[key] = kv.Value.Bytes
		} else { // uint
			state[key] = kv.Value.Uint
		}
	}

	return state, nil
}

// GetAccountBalance returns the ALGO balance of an address
func (s *AlgorandService) GetAccountBalance(ctx context.Context, address string) (uint64, error) {
	info, err := s.algodClient.AccountInformation(address).Do(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get account info: %w", err)
	}
	return info.Amount, nil
}

// Helper: encode transaction to msgpack bytes
func encodeMsgpack(txn types.Transaction) []byte {
	return msgpack.Encode(txn)
}

// Helper: convert uint64 to big-endian bytes
func uint64ToBytes(val uint64) []byte {
	b := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		b[i] = byte(val & 0xff)
		val >>= 8
	}
	return b
}
