package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ========================
// Database Models (v3.1)
// ========================

// Profile represents a user profile
type Profile struct {
	ID                     uuid.UUID `json:"id" db:"id"`
	ClerkID                string    `json:"clerk_id" db:"clerk_id"`
	Username               string    `json:"username" db:"username"`
	DisplayName            *string   `json:"display_name" db:"display_name"`
	Email                  string    `json:"email" db:"email"`
	AvatarURL              *string   `json:"avatar_url" db:"avatar_url"` // Cloudflare R2 URL
	WalletAddress          *string   `json:"wallet_address" db:"wallet_address"`
	Role                   string    `json:"role" db:"role"` // Default: "freelancer"
	Bio                    *string   `json:"bio" db:"bio"`
	ReputationScore        int       `json:"reputation_score" db:"reputation_score"`
	TotalBountiesCreated   int       `json:"total_bounties_created" db:"total_bounties_created"`
	TotalBountiesCompleted int       `json:"total_bounties_completed" db:"total_bounties_completed"`
	TotalEarnedAlgo        float64   `json:"total_earned_algo" db:"total_earned_algo"`
	StreakCount             int       `json:"streak_count" db:"streak_count"`
	AvgRating              float64   `json:"avg_rating" db:"avg_rating"`
	TotalRatings           int       `json:"total_ratings" db:"total_ratings"`
	CreatedAt              time.Time `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time `json:"updated_at" db:"updated_at"`
}

// Bounty represents a bounty record (v3.3)
type Bounty struct {
	ID                   uuid.UUID  `json:"id" db:"id"`
	BountyID             string     `json:"bounty_id" db:"bounty_id"` // CR00847 format
	CreatorID            uuid.UUID  `json:"creator_id" db:"creator_id"`
	Title                string     `json:"title" db:"title"`
	Description          string     `json:"description" db:"description"`
	RewardAlgo           float64    `json:"reward_algo" db:"reward_algo"`
	Deadline             time.Time  `json:"deadline" db:"deadline"`
	Status               string     `json:"status" db:"status"`
	MaxSubmissions       int        `json:"max_submissions" db:"max_submissions"`
	SubmissionsRemaining int        `json:"submissions_remaining" db:"submissions_remaining"`
	Tags                 []string   `json:"tags" db:"tags"`
	AppID                *int64     `json:"app_id" db:"app_id"` // Algorand app ID (per-bounty deployment)
	EscrowTxnID          *string    `json:"escrow_txn_id" db:"escrow_txn_id"`
	PayoutTxnID          *string    `json:"payout_txn_id" db:"payout_txn_id"`
	AcceptedFreelancerID *uuid.UUID `json:"accepted_freelancer_id" db:"accepted_freelancer_id"` // v3.3
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`

	// Joined fields
	Creator         *Profile `json:"creator,omitempty"`
	SubmissionCount int      `json:"submission_count"`
}

// Submission represents a work submission (v3.3)
type Submission struct {
	ID                  uuid.UUID  `json:"id" db:"id"`
	BountyID            uuid.UUID  `json:"bounty_id" db:"bounty_id"`
	FreelancerID        uuid.UUID  `json:"freelancer_id" db:"freelancer_id"`
	SubmissionNumber    int        `json:"submission_number" db:"submission_number"`
	FileURL             *string    `json:"file_url" db:"file_url"`           // Legacy R2 path (nullable now)
	FileType            *string    `json:"file_type" db:"file_type"`         // Legacy
	FileSizeBytes       *int       `json:"file_size_bytes" db:"file_size_bytes"` // Legacy
	MegaNZLink          string     `json:"mega_nz_link" db:"mega_nz_link"`   // v3.3: mega.nz download link
	EncryptionKeyR2Path *string    `json:"encryption_key_r2_path" db:"encryption_key_r2_path"` // v3.3: R2 path to .txt key file
	EncryptionKeyR2URL  *string    `json:"encryption_key_r2_url" db:"encryption_key_r2_url"`   // v3.3: presigned URL
	Description         string     `json:"description" db:"description"`
	Status              string     `json:"status" db:"status"`
	RejectionFeedback   *string    `json:"rejection_feedback" db:"rejection_feedback"` // Min 50 chars
	CreatorMessage      *string    `json:"creator_message" db:"creator_message"`
	CreatorRating       *int       `json:"creator_rating" db:"creator_rating"` // 1-5
	SubmissionTxnID     *string    `json:"submission_txn_id" db:"submission_txn_id"`
	WorkHashSHA256      string     `json:"work_hash_sha256" db:"work_hash_sha256"` // On-chain proof
	ReviewedAt          *time.Time `json:"reviewed_at" db:"reviewed_at"`
	ResolvedAt          *time.Time `json:"resolved_at" db:"resolved_at"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`

	// Joined
	Freelancer    *Profile `json:"freelancer,omitempty"`
	Bounty        *Bounty  `json:"bounty,omitempty"`
	SignedFileURL string   `json:"signed_file_url,omitempty"` // R2 pre-signed URL (24hr) for encryption key
}

// SubmissionHistoryEntry for dispute submission_history JSONB
type SubmissionHistoryEntry struct {
	Attempt           int    `json:"attempt"`
	FileR2Path        string `json:"file_r2_path"`
	Description       string `json:"description"`
	RejectionFeedback string `json:"rejection_feedback"`
	SubmittedAt       string `json:"submitted_at"`
	RejectedAt        string `json:"rejected_at,omitempty"`
}

// Dispute represents a v3.1 DAO Court dispute
type Dispute struct {
	ID                    uuid.UUID              `json:"id" db:"id"`
	DisputeID             string                 `json:"dispute_id" db:"dispute_id"` // DSP004821 format
	BountyID              uuid.UUID              `json:"bounty_id" db:"bounty_id"`
	FreelancerID          uuid.UUID              `json:"freelancer_id" db:"freelancer_id"`
	CreatorID             uuid.UUID              `json:"creator_id" db:"creator_id"`
	FreelancerDescription string                 `json:"freelancer_description" db:"freelancer_description"`
	SubmissionHistory     json.RawMessage        `json:"submission_history" db:"submission_history"`
	Status                string                 `json:"status" db:"status"`
	VotesCreator          int                    `json:"votes_creator" db:"votes_creator"`
	VotesFreelancer       int                    `json:"votes_freelancer" db:"votes_freelancer"`
	VotingDeadline        time.Time              `json:"voting_deadline" db:"voting_deadline"`
	ResolvedAt            *time.Time             `json:"resolved_at" db:"resolved_at"`
	ResolutionTxnID       *string                `json:"resolution_txn_id" db:"resolution_txn_id"`
	IPFSDisputeCID        *string                `json:"ipfs_dispute_cid" db:"ipfs_dispute_cid"`
	CreatedAt             time.Time              `json:"created_at" db:"created_at"`

	// Joined
	Bounty     *Bounty  `json:"bounty,omitempty"`
	Freelancer *Profile `json:"freelancer,omitempty"`
	Creator    *Profile `json:"creator,omitempty"`
}

// DAOVote represents a DAO court vote
type DAOVote struct {
	ID          uuid.UUID `json:"id" db:"id"`
	DisputeID   uuid.UUID `json:"dispute_id" db:"dispute_id"`
	VoterID     uuid.UUID `json:"voter_id" db:"voter_id"`
	Vote        string    `json:"vote" db:"vote"` // "creator" or "freelancer"
	VoteTxnID   *string   `json:"vote_txn_id" db:"vote_txn_id"`
	IPFSVoteCID *string   `json:"ipfs_vote_cid" db:"ipfs_vote_cid"`
	VotedAt     time.Time `json:"voted_at" db:"voted_at"`

	// Joined
	Voter *Profile `json:"voter,omitempty"`
}

// TransactionLog represents an immutable on-chain event record
type TransactionLog struct {
	ID              uuid.UUID   `json:"id" db:"id"`
	BountyID        *uuid.UUID  `json:"bounty_id" db:"bounty_id"`
	ActorID         *uuid.UUID  `json:"actor_id" db:"actor_id"`
	Event           string      `json:"event" db:"event"`
	TxnID           *string     `json:"txn_id" db:"txn_id"`
	TxnNote         *string     `json:"txn_note" db:"txn_note"`
	IPFSMetadataCID *string     `json:"ipfs_metadata_cid" db:"ipfs_metadata_cid"`
	IPFSGatewayURL  *string     `json:"ipfs_gateway_url" db:"ipfs_gateway_url"`
	AmountAlgo      *float64    `json:"amount_algo" db:"amount_algo"`
	Metadata        interface{} `json:"metadata,omitempty" db:"metadata"`
	CreatedAt       time.Time   `json:"created_at" db:"created_at"`

	// Joined
	Actor  *Profile `json:"actor,omitempty"`
	Bounty *Bounty  `json:"bounty,omitempty"`
}

// AdminUser represents the admin panel user
type AdminUser struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	Username     string     `json:"username" db:"username"`
	PasswordHash string     `json:"-" db:"password_hash"`
	DisplayName  *string    `json:"display_name" db:"display_name"`
	LastLoginAt  *time.Time `json:"last_login_at" db:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

// AdminStats holds aggregated platform-wide statistics
type AdminStats struct {
	TotalFreelancers     int     `json:"total_freelancers"`
	TotalCreators        int     `json:"total_creators"`
	TotalBounties        int     `json:"total_bounties"`
	OpenBounties         int     `json:"open_bounties"`
	InProgressBounties   int     `json:"in_progress_bounties"`
	CompletedBounties    int     `json:"completed_bounties"`
	DisputedBounties     int     `json:"disputed_bounties"`
	TotalSubmissions     int     `json:"total_submissions"`
	AcceptedSubmissions  int     `json:"accepted_submissions"`
	TotalAlgoVolume      float64 `json:"total_algo_volume"`
	TotalAlgoPaidOut     float64 `json:"total_algo_paid_out"`
	ActiveDisputes       int     `json:"active_disputes"`
	TotalTransactions    int     `json:"total_transactions"`
}

// AuditLogEntry records admin actions
type AuditLogEntry struct {
	ID            uuid.UUID   `json:"id" db:"id"`
	AdminID       *uuid.UUID  `json:"admin_id" db:"admin_id"`
	AdminUsername string      `json:"admin_username" db:"admin_username"`
	Action        string      `json:"action" db:"action"`
	TargetType    *string     `json:"target_type" db:"target_type"`
	TargetID      *string     `json:"target_id" db:"target_id"`
	OldValue      interface{} `json:"old_value,omitempty" db:"old_value"`
	NewValue      interface{} `json:"new_value,omitempty" db:"new_value"`
	IPAddress     *string     `json:"ip_address" db:"ip_address"`
	CreatedAt     time.Time   `json:"created_at" db:"created_at"`
}

// ========================
// Request DTOs (v3.1)
// ========================

type SyncProfileRequest struct {
	Email     string `json:"email"`
	Username  string `json:"username"`
	Role      string `json:"role"` // defaults to "freelancer" if empty
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type UpdateProfileRequest struct {
	DisplayName   *string `json:"display_name"`
	Bio           *string `json:"bio"`
	WalletAddress *string `json:"wallet_address"`
}

type CreateBountyRequest struct {
	Title          string   `json:"title" binding:"required,min=5,max=300"`
	Description    string   `json:"description" binding:"required,min=20"`
	RewardAlgo     float64  `json:"reward_algo" binding:"required,min=1"`
	Deadline       string   `json:"deadline" binding:"required"`
	MaxSubmissions int      `json:"max_submissions" binding:"required,min=1,max=50"`
	Tags           []string `json:"tags"`
}

type LockBountyRequest struct {
	BountyID      string `json:"bounty_id"`
	WalletAddress string `json:"wallet_address" binding:"required"`
}

type ConfirmLockRequest struct {
	BountyID    string   `json:"bounty_id"`
	SignedTxns  []string `json:"signed_txns" binding:"required"`
	AppID       int64    `json:"app_id"`
}

type ApproveSubmissionRequest struct {
	SubmissionID  string  `json:"submission_id" binding:"required,uuid"`
	Message       string  `json:"message"` // Optional creator message
	Rating        int     `json:"rating" binding:"required,min=1,max=5"` // 1-5 stars
	SignedTxns    []string `json:"signed_txns"`
}

type RejectSubmissionRequest struct {
	SubmissionID string   `json:"submission_id" binding:"required,uuid"`
	Feedback     string   `json:"feedback" binding:"required,min=50"` // Min 50 chars enforced
	SignedTxns   []string `json:"signed_txns"`
}

type RaiseDisputeRequest struct {
	Description string   `json:"description" binding:"required"` // Min 300 words enforced server-side
	SignedTxns  []string `json:"signed_txns"`
}

type LetGoRequest struct {
	SignedTxns []string `json:"signed_txns"`
}

type CastDAOVoteRequest struct {
	Vote       string   `json:"vote" binding:"required,oneof=creator freelancer"`
	SignedTxns []string `json:"signed_txns" binding:"required"`
}

// SubmitWorkV3Request is the v3.3 submission request (mega.nz + encryption key)
type SubmitWorkV3Request struct {
	MegaNZLink  string `json:"mega_nz_link" binding:"required"`  // User-provided mega.nz link
	Description string `json:"description" binding:"required"`   // Work description
	// Encryption key .txt file is sent via multipart form field "encryption_key"
}

type AdminLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type AdminLoginResponse struct {
	Token   string     `json:"token"`
	Admin   AdminUser  `json:"admin"`
	Message string     `json:"message"`
}

type UpdateBountyStatusRequest struct {
	Status string `json:"status" binding:"required"`
	Note   string `json:"note"`
}

// ========================
// Response DTOs
// ========================

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type PaginatedResponse struct {
	Items      interface{} `json:"items"`
	TotalCount int         `json:"total_count"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// DashboardStats is returned for user-specific stats
type DashboardStats struct {
	Role                   string  `json:"role"`
	ReputationScore        int     `json:"reputation_score"`
	TotalBountiesCreated   int     `json:"total_bounties_created"`
	TotalBountiesCompleted int     `json:"total_bounties_completed"`
	TotalEarningsAlgo      float64 `json:"total_earnings_algo"`
	StreakCount             int     `json:"streak_count"`
	AvgRating              float64 `json:"avg_rating"`
	TotalRatings           int     `json:"total_ratings"`
	ActiveBounties         int     `json:"active_bounties"`
	ActiveSubmissions      int     `json:"active_submissions"`
	TotalSubmissions       int     `json:"total_submissions"`
	ApprovedSubmissions    int     `json:"approved_submissions"`
	WinRate                float64 `json:"win_rate"`
	UnreadNotifications    int     `json:"unread_notifications"`
	TotalSpentAlgo         float64 `json:"total_spent_algo"`
	OpenDisputes           int     `json:"open_disputes"`
	WorkingBounties        int     `json:"working_bounties"`
	PendingAcceptances     int     `json:"pending_acceptances"`
}

// BountyAcceptance represents a freelancer's request to accept/work on a bounty
type BountyAcceptance struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	BountyID     uuid.UUID  `json:"bounty_id" db:"bounty_id"`
	FreelancerID uuid.UUID  `json:"freelancer_id" db:"freelancer_id"`
	Status       string     `json:"status" db:"status"` // pending, approved, rejected
	Message      *string    `json:"message" db:"message"`
	CreatorNote  *string    `json:"creator_note" db:"creator_note"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`

	// Joined
	Freelancer *Profile `json:"freelancer,omitempty"`
	Bounty     *Bounty  `json:"bounty,omitempty"`
}

// AcceptBountyRequest is sent by a freelancer to request acceptance on a bounty
type AcceptBountyRequest struct {
	Message string `json:"message"` // optional cover message
}

// ReviewAcceptanceRequest is sent by a creator to approve/reject a freelancer's acceptance
type ReviewAcceptanceRequest struct {
	FreelancerID  string `json:"freelancer_id" binding:"required"`
	Action        string `json:"action" binding:"required"` // "approve" or "reject"
	Note          string `json:"note"`                      // optional note
	WalletAddress string `json:"wallet_address"`            // required for approve (escrow lock)
}

// ConfirmAcceptanceRequest is sent after Pera signs the escrow txns
type ConfirmAcceptanceRequest struct {
	FreelancerID string   `json:"freelancer_id" binding:"required"`
	SignedTxns   []string `json:"signed_txns" binding:"required"`
	AppID        int64    `json:"app_id"`
}
