package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// IPFSService handles Pinata IPFS operations
// IMPORTANT: IPFS is used EXCLUSIVELY for transaction metadata JSON.
// It does NOT store submission files, bounty terms, or any binary content.
// One IPFS pin per significant on-chain event.
type IPFSService struct {
	jwt        string
	gatewayURL string
	httpClient *http.Client
}

// NewIPFSService creates a new IPFS service instance
func NewIPFSService(pinataJWT, gatewayURL string) *IPFSService {
	return &IPFSService{
		jwt:        pinataJWT,
		gatewayURL: gatewayURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ============================================================
// IPFS Transaction Metadata Schemas (v3.1)
// One struct per on-chain event type
// ============================================================

// TxnEvent represents an on-chain event type
type TxnEvent string

const (
	EventBountyCreated      TxnEvent = "bounty_created"
	EventEscrowLocked       TxnEvent = "escrow_locked"
	EventWorkSubmitted      TxnEvent = "work_submitted"
	EventSubmissionApproved TxnEvent = "submission_approved"
	EventSubmissionRejected TxnEvent = "submission_rejected"
	EventDisputeRaised      TxnEvent = "dispute_raised"
	EventDAOVoteCast        TxnEvent = "dao_vote_cast"
	EventDAOResolved        TxnEvent = "dao_resolved"
	EventBountyRefunded     TxnEvent = "bounty_refunded"
	EventBountyCancelled    TxnEvent = "bounty_cancelled"
	EventBountyExpired      TxnEvent = "bounty_expired"
	EventFreelancerLetGo    TxnEvent = "freelancer_letgo"
)

// BountyCreatedMetadata is pinned to IPFS when a bounty is locked on-chain
type BountyCreatedMetadata struct {
	Event          string    `json:"event"`
	BountyID       string    `json:"bounty_id"`
	AppID          int64     `json:"app_id"`
	CreatorAddress string    `json:"creator_address"`
	RewardAlgo     float64   `json:"reward_algo"`
	MaxSubmissions int       `json:"max_submissions"`
	Deadline       string    `json:"deadline"`
	Tags           []string  `json:"tags"`
	TxnID          string    `json:"txn_id"`
	Timestamp      string    `json:"timestamp"`
	Platform       string    `json:"platform"`
	Network        string    `json:"network"`
}

// WorkSubmittedMetadata is pinned to IPFS when a freelancer submits work
type WorkSubmittedMetadata struct {
	Event              string  `json:"event"`
	BountyID           string  `json:"bounty_id"`
	SubmissionNumber   int     `json:"submission_number"`
	FreelancerAddress  string  `json:"freelancer_address"`
	FileR2Path         string  `json:"file_r2_path"`
	FileType           string  `json:"file_type"`
	FileSizeBytes      int64   `json:"file_size_bytes"`
	DescriptionPreview string  `json:"description_preview"` // First 200 chars
	WorkHashSHA256     string  `json:"work_hash_sha256"`
	TxnID              string  `json:"txn_id"`
	Timestamp          string  `json:"timestamp"`
	Platform           string  `json:"platform"`
	Network            string  `json:"network"`
}

// SubmissionApprovedMetadata is pinned to IPFS on creator approval + payout
type SubmissionApprovedMetadata struct {
	Event             string  `json:"event"`
	BountyID          string  `json:"bounty_id"`
	SubmissionID      string  `json:"submission_id"`
	FreelancerAddress string  `json:"freelancer_address"`
	CreatorAddress    string  `json:"creator_address"`
	RewardPaidAlgo    float64 `json:"reward_paid_algo"`
	CreatorRating     int     `json:"creator_rating,omitempty"`
	CreatorMessage    string  `json:"creator_message,omitempty"`
	PayoutTxnID       string  `json:"payout_txn_id"`
	Timestamp         string  `json:"timestamp"`
	Platform          string  `json:"platform"`
	Network           string  `json:"network"`
}

// SubmissionRejectedMetadata is pinned to IPFS on creator rejection
type SubmissionRejectedMetadata struct {
	Event                    string `json:"event"`
	BountyID                 string `json:"bounty_id"`
	SubmissionID             string `json:"submission_id"`
	RejectionNumber          int    `json:"rejection_number"`
	SubmissionsRemainingAfter int   `json:"submissions_remaining_after"`
	FeedbackPreview          string `json:"feedback_preview"` // First 200 chars
	TxnID                    string `json:"txn_id"`
	Timestamp                string `json:"timestamp"`
	Platform                 string `json:"platform"`
	Network                  string `json:"network"`
}

// SubmissionHistoryEntry for dispute metadata
type SubmissionHistoryEntry struct {
	Attempt           int    `json:"attempt"`
	FileR2Path        string `json:"file_r2_path"`
	Description       string `json:"description"`
	RejectionFeedback string `json:"rejection_feedback"`
	SubmittedAt       string `json:"submitted_at"`
	RejectedAt        string `json:"rejected_at,omitempty"`
}

// DisputeRaisedMetadata is pinned to IPFS when freelancer raises a dispute
type DisputeRaisedMetadata struct {
	Event               string                   `json:"event"`
	DisputeID           string                   `json:"dispute_id"`
	BountyID            string                   `json:"bounty_id"`
	FreelancerAddress   string                   `json:"freelancer_address"`
	DisputeDescription  string                   `json:"dispute_description"` // Full 300+ word text
	SubmissionHistory   []SubmissionHistoryEntry `json:"submission_history"`
	VotingDeadline      string                   `json:"voting_deadline"`
	TxnID               string                   `json:"txn_id"`
	Timestamp           string                   `json:"timestamp"`
	Platform            string                   `json:"platform"`
	Network             string                   `json:"network"`
}

// DAOVoteCastMetadata is pinned to IPFS per DAO vote
type DAOVoteCastMetadata struct {
	Event                  string `json:"event"`
	DisputeID              string `json:"dispute_id"`
	BountyID               string `json:"bounty_id"`
	VoterAddress           string `json:"voter_address"`
	VoteFor                string `json:"vote_for"` // "creator" or "freelancer"
	VotesCreatorAfter      int    `json:"votes_creator_after"`
	VotesFreelancerAfter   int    `json:"votes_freelancer_after"`
	TxnID                  string `json:"txn_id"`
	Timestamp              string `json:"timestamp"`
	Platform               string `json:"platform"`
	Network                string `json:"network"`
}

// DAODisputeResolvedMetadata is pinned to IPFS when DAO resolution executes
type DAODisputeResolvedMetadata struct {
	Event               string  `json:"event"`
	DisputeID           string  `json:"dispute_id"`
	BountyID            string  `json:"bounty_id"`
	Winner              string  `json:"winner"` // "creator" or "freelancer"
	FinalVotesCreator   int     `json:"final_votes_creator"`
	FinalVotesFreelancer int    `json:"final_votes_freelancer"`
	AlgoReleasedTo      string  `json:"algo_released_to"`
	AlgoAmount          float64 `json:"algo_amount"`
	ResolutionTxnID     string  `json:"resolution_txn_id"`
	Timestamp           string  `json:"timestamp"`
	Platform            string  `json:"platform"`
	Network             string  `json:"network"`
}

// FreelancerLetGoMetadata is pinned when freelancer forfeits
type FreelancerLetGoMetadata struct {
	Event             string  `json:"event"`
	BountyID          string  `json:"bounty_id"`
	FreelancerAddress string  `json:"freelancer_address"`
	CreatorAddress    string  `json:"creator_address"`
	RefundedAlgo      float64 `json:"refunded_algo"`
	RatingReset       bool    `json:"rating_reset"` // v3.3: freelancer ratings set to 0
	TxnID             string  `json:"txn_id"`
	Timestamp         string  `json:"timestamp"`
	Platform          string  `json:"platform"`
	Network           string  `json:"network"`
}

// BountyAcceptedMetadata is pinned when creator accepts freelancer and deploys escrow (v3.3)
type BountyAcceptedMetadata struct {
	Event              string  `json:"event"`
	BountyID           string  `json:"bounty_id"`
	AppID              int64   `json:"app_id"`
	CreatorAddress     string  `json:"creator_address"`
	FreelancerAddress  string  `json:"freelancer_address"`
	RewardAlgo         float64 `json:"reward_algo"`
	MaxSubmissions     int     `json:"max_submissions"`
	Deadline           string  `json:"deadline"`
	EscrowTxnID        string  `json:"escrow_txn_id"`
	Timestamp          string  `json:"timestamp"`
	Platform           string  `json:"platform"`
	Network            string  `json:"network"`
}

// WorkResubmittedMetadata is pinned when freelancer resubmits after rejection (v3.3)
type WorkResubmittedMetadata struct {
	Event              string `json:"event"`
	BountyID           string `json:"bounty_id"`
	SubmissionNumber   int    `json:"submission_number"`
	FreelancerAddress  string `json:"freelancer_address"`
	MegaNZLink         string `json:"mega_nz_link"`
	DescriptionPreview string `json:"description_preview"`
	WorkHashSHA256     string `json:"work_hash_sha256"`
	TxnID              string `json:"txn_id"`
	Timestamp          string `json:"timestamp"`
	Platform           string `json:"platform"`
	Network            string `json:"network"`
}

// ============================================================
// Pinata API Implementation
// ============================================================

// PinataResponse is the response from Pinata API
type PinataResponse struct {
	IpfsHash  string `json:"IpfsHash"`
	PinSize   int    `json:"PinSize"`
	Timestamp string `json:"Timestamp"`
}

// PinResult contains the result of a successful IPFS pin
type PinResult struct {
	CID        string `json:"cid"`
	GatewayURL string `json:"gateway_url"` // Full gateway URL for UI display
	PinSize    int    `json:"pin_size"`
}

// PinJSON pins a metadata object to IPFS via Pinata
// This is fire-and-forget safe: failures are logged but never block transaction flow
func (s *IPFSService) PinJSON(ctx context.Context, name string, data interface{}) (*PinResult, error) {
	payload := map[string]interface{}{
		"pinataContent": data,
		"pinataMetadata": map[string]interface{}{
			"name": fmt.Sprintf("BountyVault-%s", name),
			"keyvalues": map[string]string{
				"platform": "BountyVault",
				"network":  "algorand-testnet",
			},
		},
		"pinataOptions": map[string]interface{}{
			"cidVersion": 1,
		},
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.pinata.cloud/pinning/pinJSONToIPFS",
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.jwt)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pinata request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pinata error (status %d): %s", resp.StatusCode, string(body))
	}

	var pinResp PinataResponse
	if err := json.NewDecoder(resp.Body).Decode(&pinResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &PinResult{
		CID:        pinResp.IpfsHash,
		GatewayURL: fmt.Sprintf("%s/%s", s.gatewayURL, pinResp.IpfsHash),
		PinSize:    pinResp.PinSize,
	}, nil
}

// PinBountyCreated pins bounty creation metadata to IPFS
func (s *IPFSService) PinBountyCreated(ctx context.Context, data BountyCreatedMetadata) (*PinResult, error) {
	data.Event = string(EventBountyCreated)
	data.Platform = "BountyVault"
	data.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return s.PinJSON(ctx, fmt.Sprintf("bounty-created-%s", data.BountyID), data)
}

// PinWorkSubmitted pins submission metadata to IPFS
func (s *IPFSService) PinWorkSubmitted(ctx context.Context, data WorkSubmittedMetadata) (*PinResult, error) {
	data.Event = string(EventWorkSubmitted)
	data.Platform = "BountyVault"
	data.Timestamp = time.Now().UTC().Format(time.RFC3339)
	// Trim description preview to 200 chars
	if len(data.DescriptionPreview) > 200 {
		data.DescriptionPreview = data.DescriptionPreview[:200] + "..."
	}
	return s.PinJSON(ctx, fmt.Sprintf("submission-%s", data.BountyID), data)
}

// PinSubmissionApproved pins approval + payout metadata to IPFS
func (s *IPFSService) PinSubmissionApproved(ctx context.Context, data SubmissionApprovedMetadata) (*PinResult, error) {
	data.Event = string(EventSubmissionApproved)
	data.Platform = "BountyVault"
	data.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return s.PinJSON(ctx, fmt.Sprintf("approved-%s-%s", data.BountyID, data.SubmissionID), data)
}

// PinSubmissionRejected pins rejection metadata to IPFS
func (s *IPFSService) PinSubmissionRejected(ctx context.Context, data SubmissionRejectedMetadata) (*PinResult, error) {
	data.Event = string(EventSubmissionRejected)
	data.Platform = "BountyVault"
	data.Timestamp = time.Now().UTC().Format(time.RFC3339)
	if len(data.FeedbackPreview) > 200 {
		data.FeedbackPreview = data.FeedbackPreview[:200] + "..."
	}
	return s.PinJSON(ctx, fmt.Sprintf("rejected-%s-%s", data.BountyID, data.SubmissionID), data)
}

// PinDisputeRaised pins dispute metadata (full 300+ word description + history) to IPFS
func (s *IPFSService) PinDisputeRaised(ctx context.Context, data DisputeRaisedMetadata) (*PinResult, error) {
	data.Event = string(EventDisputeRaised)
	data.Platform = "BountyVault"
	data.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return s.PinJSON(ctx, fmt.Sprintf("dispute-%s", data.DisputeID), data)
}

// PinDAOVoteCast pins each DAO vote to IPFS
func (s *IPFSService) PinDAOVoteCast(ctx context.Context, data DAOVoteCastMetadata) (*PinResult, error) {
	data.Event = string(EventDAOVoteCast)
	data.Platform = "BountyVault"
	data.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return s.PinJSON(ctx, fmt.Sprintf("vote-%s-%s", data.DisputeID, computeShortHash(data.VoterAddress)), data)
}

// PinDAOResolved pins DAO resolution outcome to IPFS
func (s *IPFSService) PinDAOResolved(ctx context.Context, data DAODisputeResolvedMetadata) (*PinResult, error) {
	data.Event = string(EventDAOResolved)
	data.Platform = "BountyVault"
	data.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return s.PinJSON(ctx, fmt.Sprintf("resolved-%s", data.DisputeID), data)
}

// PinFreelancerLetGo pins let-go metadata to IPFS
func (s *IPFSService) PinFreelancerLetGo(ctx context.Context, data FreelancerLetGoMetadata) (*PinResult, error) {
	data.Event = string(EventFreelancerLetGo)
	data.Platform = "BountyVault"
	data.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return s.PinJSON(ctx, fmt.Sprintf("letgo-%s", data.BountyID), data)
}

// PinBountyAccepted pins acceptance metadata when creator accepts freelancer (v3.3)
func (s *IPFSService) PinBountyAccepted(ctx context.Context, data BountyAcceptedMetadata) (*PinResult, error) {
	data.Event = "bounty_accepted"
	data.Platform = "BountyVault"
	data.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return s.PinJSON(ctx, fmt.Sprintf("accepted-%s", data.BountyID), data)
}

// PinWorkResubmitted pins resubmission metadata (v3.3)
func (s *IPFSService) PinWorkResubmitted(ctx context.Context, data WorkResubmittedMetadata) (*PinResult, error) {
	data.Event = "work_resubmitted"
	data.Platform = "BountyVault"
	data.Timestamp = time.Now().UTC().Format(time.RFC3339)
	if len(data.DescriptionPreview) > 200 {
		data.DescriptionPreview = data.DescriptionPreview[:200] + "..."
	}
	return s.PinJSON(ctx, fmt.Sprintf("resubmit-%s-%d", data.BountyID, data.SubmissionNumber), data)
}

// GetGatewayURL returns the public gateway URL for a given CID
func (s *IPFSService) GetGatewayURL(cid string) string {
	return fmt.Sprintf("%s/%s", s.gatewayURL, cid)
}

// IsConfigured returns true if Pinata JWT is set
func (s *IPFSService) IsConfigured() bool {
	return s.jwt != "" && s.jwt != "your-pinata-jwt-token"
}

func computeShortHash(input string) string {
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:4])
}
