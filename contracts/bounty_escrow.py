# ============================================================
# BountyVault — Smart Contract (ARC4 ABI / Algorand Python)
# Decentralized Bounty Escrow on Algorand Testnet
# PRD v3.3 — AlgoBharat Hackathon | April 2026
#
# v3.3 Architecture Changes:
#   - Per-bounty contract deployment (~0.1 ALGO per bounty)
#   - Escrow = Contract App Account (no external wallet)
#   - Dispute resolution = DAO Court (community votes, 48hr window)
#   - Tie in DAO voting → Creator wins (refund)
#   - submit_proof requires mega_link_hash (validates mega.nz link given)
#   - reject_submission resets worker_status → allows resubmission
#   - let_go_bounty() — freelancer forfeits (rating reset off-chain)
#   - cast_dao_vote() — community votes on disputes
#   - resolve_dao_dispute() — permissionless, after dao_deadline
#   - submissions_remaining tracks per-bounty rejection budget
#   - Auto-refund when submissions_remaining hits 0 on last rejection
# ============================================================

from algopy import (
    ARC4Contract,
    Account,
    Bytes,
    Global,
    Txn,
    UInt64,
    arc4,
    gtxn,
    itxn,
    op,
    subroutine,
)
from algopy.arc4 import abimethod, String as ARC4String

# ========================
# Status Constants
# ========================
STATUS_OPEN        = UInt64(0)  # funded, awaiting submissions
STATUS_IN_PROGRESS = UInt64(1)  # at least one submission received
STATUS_COMPLETED   = UInt64(2)  # payout done
STATUS_DISPUTED    = UInt64(3)  # DAO court active
STATUS_EXPIRED     = UInt64(4)  # deadline passed or all rejections exhausted
STATUS_CANCELLED   = UInt64(5)  # creator cancelled (0 submissions)

# Worker local state
WORKER_NONE      = UInt64(0)
WORKER_SUBMITTED = UInt64(1)
WORKER_ACCEPTED  = UInt64(2)
WORKER_REJECTED  = UInt64(3)

# DAO vote choices
VOTE_NONE        = UInt64(0)
VOTE_CREATOR     = UInt64(1)
VOTE_FREELANCER  = UInt64(2)

# Time constants
FORTY_EIGHT_HOURS = UInt64(172800)  # 48 hours for DAO voting window
ONE_HOUR          = UInt64(3600)    # 1 hour minimum deadline
MIN_REWARD        = UInt64(1000000) # 1 ALGO minimum reward


class BountyEscrow(ARC4Contract):
    """
    ARC4-compliant Bounty Escrow Smart Contract — v3.1

    The contract App Account IS the escrow.
    ALGO flows:
      Creator → App Account (via grouped Payment + create_bounty)
      App Account → Freelancer (approve_payout or resolve_dao_dispute freelancer win)
      App Account → Creator (reject all submissions, let_go_bounty, dao tie/creator win,
                              refund_expired, cancel_bounty)

    No arbitrator. No external wallet. Fully trustless.
    """

    # ========================
    # Global State (on-chain)
    # ========================
    creator: Account            # Bounty creator address
    reward: UInt64              # Reward in microALGOs
    deadline: UInt64            # Unix timestamp — submission deadline
    dao_deadline: UInt64        # Unix timestamp — DAO voting deadline (set when dispute raised)
    status: UInt64              # Bounty status enum (0-5)
    max_submissions: UInt64     # Creator-set maximum (1-50)
    submissions_remaining: UInt64 # Starts at max_submissions, decremented per rejection
    submission_count: UInt64    # Total submissions ever received
    approved_freelancer: Account # Set on approve_payout
    votes_creator: UInt64       # DAO vote tally
    votes_freelancer: UInt64    # DAO vote tally
    dispute_freelancer: Account  # Freelancer who raised dispute

    # ========================
    # Local State (per opted-in user)
    # work_hash: Bytes(32)       SHA-256 of R2 file path + file size
    # submission_time: UInt64    Unix timestamp
    # worker_status: UInt64      0=none 1=submitted 2=accepted 3=rejected
    # has_voted: UInt64          0=not voted 1=voted
    # vote_choice: UInt64        0=none 1=creator 2=freelancer
    # ========================

    # ========================
    # ABI Methods — Core
    # ========================

    @abimethod()
    def create_bounty(
        self,
        payment: gtxn.PaymentTransaction,
        deadline: UInt64,
        max_submissions: UInt64,
    ) -> ARC4String:
        """
        Create a new bounty with locked ALGO reward.
        Must be called as part of an atomic group:
          Txn 0: Payment(sender=creator, receiver=app_address, amount=reward_microalgo)
          Txn 1: ApplicationCall(method=create_bounty)

        Note format: 'BountyVault:bounty_created:{app_id}'
        """
        # Validate payment receiver is this contract
        assert payment.receiver == Global.current_application_address, "Payment must go to escrow"
        # Validate minimum reward
        assert payment.amount >= MIN_REWARD, "Minimum reward is 1 ALGO"
        # Validate deadline at least 1 hour from now
        assert deadline > Global.latest_timestamp + ONE_HOUR, "Deadline must be at least 1 hour from now"
        # Validate max_submissions bounds
        assert max_submissions >= UInt64(1) and max_submissions <= UInt64(50), "max_submissions must be 1-50"

        # Initialize global state
        self.creator = Txn.sender
        self.reward = payment.amount
        self.deadline = deadline
        self.dao_deadline = UInt64(0)
        self.status = STATUS_OPEN
        self.max_submissions = max_submissions
        self.submissions_remaining = max_submissions
        self.submission_count = UInt64(0)
        self.approved_freelancer = Global.zero_address
        self.votes_creator = UInt64(0)
        self.votes_freelancer = UInt64(0)
        self.dispute_freelancer = Global.zero_address

        return ARC4String("bounty_created")

    @abimethod(allow_actions=["OptIn"])
    def opt_in(self) -> ARC4String:
        """
        Initialize local state for a user — freelancers before submitting,
        voters before casting DAO vote.
        """
        op.app_local_put(Txn.sender, Bytes(b"work_hash"), Bytes(b""))
        op.app_local_put(Txn.sender, Bytes(b"submission_time"), UInt64(0))
        op.app_local_put(Txn.sender, Bytes(b"worker_status"), WORKER_NONE)
        op.app_local_put(Txn.sender, Bytes(b"has_voted"), UInt64(0))
        op.app_local_put(Txn.sender, Bytes(b"vote_choice"), VOTE_NONE)
        return ARC4String("opted_in")

    @abimethod()
    def submit_proof(self, work_hash: Bytes, mega_link_hash: Bytes) -> ARC4String:
        """
        Submit work proof for the bounty.
        work_hash = SHA256(mega_nz_link + encryption_key_r2_path) — computed in backend.
        mega_link_hash = SHA256(mega_nz_link) — proves mega.nz link was provided.

        Validates:
        - Status is OPEN or IN_PROGRESS
        - Deadline not passed
        - submissions_remaining > 0
        - Sender is not creator
        - Worker has not already submitted (WORKER_NONE state)
        - mega_link_hash is non-empty (mega.nz link must be given)

        Note format: 'BountyVault:work_submitted:{app_id}'
        """
        assert self.status == STATUS_OPEN or self.status == STATUS_IN_PROGRESS, "Bounty not accepting submissions"
        assert Global.latest_timestamp <= self.deadline, "Deadline has passed"
        assert self.submissions_remaining > UInt64(0), "No submission slots remaining"
        assert Txn.sender != self.creator, "Creator cannot submit work"
        assert work_hash.length == UInt64(32), "work_hash must be 32 bytes"
        # Validate mega.nz link was provided (hash must be 32 bytes = SHA256)
        assert mega_link_hash.length == UInt64(32), "mega_link_hash must be 32 bytes (SHA256 of mega.nz link)"

        worker_status_val = op.app_local_get(Txn.sender, Bytes(b"worker_status"))
        # Allow submission if worker is NONE (first time) or was reset after rejection
        assert worker_status_val == WORKER_NONE, "Already submitted — wait for review or rejection reset"

        # Record submission in local state
        op.app_local_put(Txn.sender, Bytes(b"work_hash"), work_hash)
        op.app_local_put(Txn.sender, Bytes(b"submission_time"), Global.latest_timestamp)
        op.app_local_put(Txn.sender, Bytes(b"worker_status"), WORKER_SUBMITTED)

        # Update global counters
        self.submission_count = self.submission_count + UInt64(1)
        if self.status == STATUS_OPEN:
            self.status = STATUS_IN_PROGRESS

        return ARC4String("work_submitted")

    @abimethod()
    def approve_payout(self, freelancer: Account) -> ARC4String:
        """
        Creator approves work and triggers ALGO payout via InnerTransaction.
        Only creator can call. Freelancer must have submitted.

        Note format: 'BountyVault:submission_approved:{app_id}'
        """
        assert Txn.sender == self.creator, "Only creator can approve"
        assert self.status == STATUS_IN_PROGRESS, "Bounty not in progress"

        worker_status_val = op.app_local_get(freelancer, Bytes(b"worker_status"))
        assert worker_status_val == WORKER_SUBMITTED, "Freelancer has not submitted"

        # Update state
        op.app_local_put(freelancer, Bytes(b"worker_status"), WORKER_ACCEPTED)
        self.approved_freelancer = freelancer
        self.status = STATUS_COMPLETED

        # Pay freelancer via InnerTransaction
        itxn.Payment(
            receiver=freelancer,
            amount=self.reward,
            fee=UInt64(0),
            note=Bytes(b"BountyVault:submission_approved"),
        ).submit()

        return ARC4String("submission_approved")

    @abimethod()
    def reject_submission(self, freelancer: Account) -> ARC4String:
        """
        Creator rejects a submission.
        Decrements submissions_remaining.
        If submissions_remaining > 0: resets worker_status to WORKER_NONE (allows resubmission).
        If submissions_remaining hits 0: sets status to EXPIRED. Does NOT auto-refund —
          freelancer must either let_go (refund creator) or initiate_dispute (DAO court).

        Note format: 'BountyVault:submission_rejected:{app_id}'
        """
        assert Txn.sender == self.creator, "Only creator can reject"
        assert self.status == STATUS_IN_PROGRESS, "Bounty not in progress"

        worker_status_val = op.app_local_get(freelancer, Bytes(b"worker_status"))
        assert worker_status_val == WORKER_SUBMITTED, "Freelancer has not submitted"

        # Decrement remaining slots
        self.submissions_remaining = self.submissions_remaining - UInt64(1)

        if self.submissions_remaining == UInt64(0):
            # All submission slots exhausted — mark as expired
            # Freelancer must choose: let_go (refund) or initiate_dispute (DAO court)
            op.app_local_put(freelancer, Bytes(b"worker_status"), WORKER_REJECTED)
            self.status = STATUS_EXPIRED
        else:
            # Still has slots remaining — reset worker to NONE so they can resubmit
            op.app_local_put(freelancer, Bytes(b"worker_status"), WORKER_NONE)

        return ARC4String("submission_rejected")

    # ========================
    # ABI Methods — DAO Court
    # ========================

    @abimethod()
    def initiate_dispute(self) -> ARC4String:
        """
        Freelancer raises a dispute after all rejections are exhausted.
        Only the rejected freelancer (worker_status=REJECTED, submissions_remaining=0) can call.
        Activates DAO Court for 48 hours.

        Note format: 'BountyVault:dispute_raised:{app_id}'
        """
        # Must be a rejected worker
        worker_status_val = op.app_local_get(Txn.sender, Bytes(b"worker_status"))
        assert worker_status_val == WORKER_REJECTED, "Only rejected freelancers can raise a dispute"
        assert self.submissions_remaining == UInt64(0), "Submission slots must be exhausted"
        # Bounty status must be EXPIRED (set when submissions_remaining hit 0)
        assert self.status == STATUS_EXPIRED, "Dispute only valid after all rejections"

        # Activate DAO Court
        self.status = STATUS_DISPUTED
        self.dao_deadline = Global.latest_timestamp + FORTY_EIGHT_HOURS
        self.votes_creator = UInt64(0)
        self.votes_freelancer = UInt64(0)
        self.dispute_freelancer = Txn.sender

        return ARC4String("dispute_raised")

    @abimethod()
    def cast_dao_vote(self, vote_for: UInt64) -> ARC4String:
        """
        Any opted-in user EXCEPT the bounty creator and the disputing freelancer can vote.
        vote_for: 1 = creator, 2 = freelancer
        One vote per user enforced at DB level (UNIQUE constraint) and here via local state.

        Note format: 'BountyVault:dao_vote_cast:{app_id}'
        """
        assert self.status == STATUS_DISPUTED, "No active dispute"
        assert Global.latest_timestamp < self.dao_deadline, "Voting period has ended"
        assert vote_for == VOTE_CREATOR or vote_for == VOTE_FREELANCER, "vote_for must be 1 (creator) or 2 (freelancer)"

        # Must not be the creator or dispute freelancer
        assert Txn.sender != self.creator, "Creator cannot vote in own dispute"
        assert Txn.sender != self.dispute_freelancer, "Disputing freelancer cannot vote"

        # No double voting
        has_voted_val = op.app_local_get(Txn.sender, Bytes(b"has_voted"))
        assert has_voted_val == UInt64(0), "Already voted"

        # Record vote in local state
        op.app_local_put(Txn.sender, Bytes(b"has_voted"), UInt64(1))
        op.app_local_put(Txn.sender, Bytes(b"vote_choice"), vote_for)

        # Tally votes
        if vote_for == VOTE_CREATOR:
            self.votes_creator = self.votes_creator + UInt64(1)
        else:
            self.votes_freelancer = self.votes_freelancer + UInt64(1)

        return ARC4String("dao_vote_cast")

    @abimethod()
    def resolve_dao_dispute(self) -> ARC4String:
        """
        Permissionless — anyone can call after dao_deadline has passed.
        Resolution logic:
          - votes_freelancer > votes_creator → pay freelancer → COMPLETED
          - tie or votes_creator >= votes_freelancer → refund creator → EXPIRED

        Note format: 'BountyVault:dao_resolved:{app_id}'
        """
        assert self.status == STATUS_DISPUTED, "No active dispute"
        assert Global.latest_timestamp >= self.dao_deadline, "Voting period not over yet"

        if self.votes_freelancer > self.votes_creator:
            # Freelancer wins — pay out
            self.status = STATUS_COMPLETED
            itxn.Payment(
                receiver=self.dispute_freelancer,
                amount=self.reward,
                fee=UInt64(0),
                note=Bytes(b"BountyVault:dao_resolved_freelancer"),
            ).submit()
            return ARC4String("dao_resolved_freelancer_wins")
        else:
            # Creator wins (tie counts as creator win) — refund
            self.status = STATUS_EXPIRED
            itxn.Payment(
                receiver=self.creator,
                amount=self.reward,
                fee=UInt64(0),
                note=Bytes(b"BountyVault:dao_resolved_creator"),
            ).submit()
            return ARC4String("dao_resolved_creator_wins")

    @abimethod()
    def let_go_bounty(self) -> ARC4String:
        """
        Freelancer voluntarily forfeits their claim.
        Only callable by the rejected freelancer (worker_status=REJECTED, submissions_remaining=0).
        Refunds creator immediately.

        Note format: 'BountyVault:freelancer_letgo:{app_id}'
        """
        worker_status_val = op.app_local_get(Txn.sender, Bytes(b"worker_status"))
        assert worker_status_val == WORKER_REJECTED, "Only rejected freelancer can let go"
        assert self.submissions_remaining == UInt64(0), "Submission slots must be exhausted"
        assert self.status == STATUS_EXPIRED, "Bounty must be in expired state"

        self.status = STATUS_CANCELLED

        # Refund creator
        itxn.Payment(
            receiver=self.creator,
            amount=self.reward,
            fee=UInt64(0),
            note=Bytes(b"BountyVault:freelancer_letgo"),
        ).submit()

        return ARC4String("freelancer_letgo")

    # ========================
    # ABI Methods — Refund & Cancel
    # ========================

    @abimethod()
    def refund_expired(self) -> ARC4String:
        """
        Permissionless — anyone can trigger if deadline has passed and bounty is OPEN or IN_PROGRESS.
        Note format: 'BountyVault:bounty_refunded:{app_id}'
        """
        assert (self.status == STATUS_OPEN or self.status == STATUS_IN_PROGRESS), "Bounty not refundable"
        assert Global.latest_timestamp > self.deadline, "Deadline not yet passed"

        self.status = STATUS_EXPIRED

        itxn.Payment(
            receiver=self.creator,
            amount=self.reward,
            fee=UInt64(0),
            note=Bytes(b"BountyVault:bounty_refunded"),
        ).submit()

        return ARC4String("bounty_refunded")

    @abimethod()
    def cancel_bounty(self) -> ARC4String:
        """
        Creator cancels bounty before any submissions.
        Note format: 'BountyVault:bounty_cancelled:{app_id}'
        """
        assert Txn.sender == self.creator, "Only creator can cancel"
        assert self.submission_count == UInt64(0), "Cannot cancel with submissions"
        assert self.status == STATUS_OPEN, "Can only cancel open bounties"

        self.status = STATUS_CANCELLED

        itxn.Payment(
            receiver=self.creator,
            amount=self.reward,
            fee=UInt64(0),
            note=Bytes(b"BountyVault:bounty_cancelled"),
        ).submit()

        return ARC4String("bounty_cancelled")

    # ========================
    # Read-Only Methods
    # ========================

    @abimethod(readonly=True)
    def get_bounty_info(self) -> arc4.Tuple[
        arc4.UInt64,  # reward (microALGOs)
        arc4.UInt64,  # deadline
        arc4.UInt64,  # status
        arc4.UInt64,  # submissions_remaining
        arc4.UInt64,  # submission_count
        arc4.UInt64,  # votes_creator
        arc4.UInt64,  # votes_freelancer
        arc4.UInt64,  # dao_deadline
    ]:
        """Get full bounty state — used by frontend to display live data."""
        return arc4.Tuple((
            arc4.UInt64(self.reward),
            arc4.UInt64(self.deadline),
            arc4.UInt64(self.status),
            arc4.UInt64(self.submissions_remaining),
            arc4.UInt64(self.submission_count),
            arc4.UInt64(self.votes_creator),
            arc4.UInt64(self.votes_freelancer),
            arc4.UInt64(self.dao_deadline),
        ))

    @abimethod(readonly=True)
    def get_voter_status(self, voter: Account) -> arc4.Tuple[
        arc4.UInt64,  # has_voted (0 or 1)
        arc4.UInt64,  # vote_choice (0=none, 1=creator, 2=freelancer)
    ]:
        """Get a specific voter's DAO vote status."""
        has_voted = op.app_local_get(voter, Bytes(b"has_voted"))
        vote_choice = op.app_local_get(voter, Bytes(b"vote_choice"))
        return arc4.Tuple((
            arc4.UInt64(has_voted),
            arc4.UInt64(vote_choice),
        ))

    @abimethod(readonly=True)
    def get_escrow_balance(self) -> arc4.UInt64:
        """Get current escrow balance held by contract app account."""
        return arc4.UInt64(op.balance(Global.current_application_address))

    @abimethod(readonly=True)
    def get_worker_info(self, worker: Account) -> arc4.Tuple[
        arc4.DynamicBytes, # work_hash
        arc4.UInt64,       # submission_time
        arc4.UInt64,       # worker_status
    ]:
        """Get a worker's submission local state."""
        work_hash = op.app_local_get(worker, Bytes(b"work_hash"))
        sub_time = op.app_local_get(worker, Bytes(b"submission_time"))
        w_status = op.app_local_get(worker, Bytes(b"worker_status"))
        return arc4.Tuple((
            arc4.DynamicBytes(work_hash),
            arc4.UInt64(sub_time),
            arc4.UInt64(w_status),
        ))
