# BountyVault — Product Requirements Document & Technical Specification

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Tech Stack](#2-tech-stack)
3. [Architecture Overview](#3-architecture-overview)
4. [Directory Structure](#4-directory-structure)
5. [System Flow & Process](#5-system-flow--process)
6. [API Endpoints](#6-api-endpoints)
7. [Smart Contract Specification](#7-smart-contract-specification)
8. [Database Schema](#8-database-schema)
9. [Security Architecture](#9-security-architecture)
10. [Implementation Workflow](#10-implementation-workflow)

---

## 1. Project Overview

### Project Name
**BountyVault** — Decentralized Bounty Escrow System

### Core Functionality
A trustless bounty platform built on Algorand blockchain that enables:
- Creators to post bounties with locked ALGO rewards
- Workers to submit work proofs via IPFS-hashed submissions
- Automatic escrow payouts through smart contracts
- Decentralized dispute resolution

### Key Value Propositions
- **Trustless**: No intermediary required — smart contracts handle escrow
- **Transparent**: All transactions immutable and auditable on-chain
- **Fast**: Algorand's 3.3s block finality
- **Cost-effective**: Transaction fees < $0.001

---

## 2. Tech Stack

### Frontend
| Technology | Version | Purpose |
|------------|---------|---------|
| **Next.js** | 16.2.1 | React framework with SSR/SSG |
| **React** | 19.2.4 | UI library |
| **TypeScript** | 5.x | Type safety |
| **Tailwind CSS** | 4.x | Styling |

### Backend
| Technology | Version | Purpose |
|------------|---------|---------|
| **Go** | Latest | REST API server |
| **Gin** | Latest | HTTP web framework |
| **go-algorand-sdk** | v2 | Algorand blockchain interaction |

### Blockchain & Storage
| Technology | Network | Purpose |
|------------|---------|---------|
| **Algorand** | Testnet | Smart contract escrow |
| **PyTeal (Puya)** | AVM v8 | Smart contract language |
| **AlgoKit** | Latest | Algorand development toolkit |
| **IPFS (Pinata)** | Cloud | Decentralized file storage |
| **Supabase** | PostgreSQL | Relational database + auth |

### Infrastructure
| Technology | Purpose |
|------------|---------|
| **Better Auth** | Authentication framework |
| **Cloudflare R2** | Avatar/media storage |
| **JWT (HS256)** | Session management |

---

## 3. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           CLIENT LAYER                                   │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    Next.js 16 Frontend                          │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │   │
│  │  │  Auth    │  │ Bounty   │  │  Work    │  │  Wallet Connect  │  │   │
│  │  │  Pages   │  │  List    │  │ Submis.  │  │  (AlgoSigner/   │  │   │
│  │  │          │  │          │  │          │  │   Pera Wallet)   │  │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ HTTPS/REST
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                           API LAYER (Go/Gin)                            │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                     Middleware Stack                              │   │
│  │  [Recovery] → [RequestID] → [Logger] → [Security] → [RateLimit] │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│  ┌───────────────────────┐  ┌───────────────────────────────────────┐   │
│  │   Auth Handler        │  │         Bounty Handler               │   │
│  │  ┌─────────────────┐  │  │  ┌────────────────────────────────┐ │   │
│  │  │ Register/Login  │  │  │  │ Create/List/Get Bounty       │ │   │
│  │  │ JWT Generation  │  │  │  │ Submit Work                  │ │   │
│  │  │ Profile Update  │  │  │  │ Approve/Reject/Dispute       │ │   │
│  │  │ Wallet Linking  │  │  │  │ Lock Funds/Confirm Lock      │ │   │
│  │  └─────────────────┘  │  │  └────────────────────────────────┘ │   │
│  └───────────────────────┘  └───────────────────────────────────────┘   │
│  ┌───────────────────────┐  ┌───────────────────────────────────────┐   │
│  │  IPFS Service        │  │        Algorand Service              │   │
│  │  ┌─────────────────┐  │  │  ┌────────────────────────────────┐ │   │
│  │  │ PinJSON()      │  │  │  │ BuildCreateBountyTxn()       │ │   │
│  │  │ PinFile()      │  │  │  │ BuildSubmitProofTxn()       │ │   │
│  │  │ SHA-256 Hash   │  │  │  │ BuildApprovePayoutTxn()     │ │   │
│  │  │ Pinata API     │  │  │  │ GetBountyInfo()              │ │   │
│  │  └─────────────────┘  │  │  │ WaitForConfirmation()        │ │   │
│  └───────────────────────┘  │  └────────────────────────────────┘ │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
           │                                       │
           ▼                                       ▼
┌───────────────────────┐               ┌───────────────────────────────┐
│    Supabase           │               │      Algorand Blockchain      │
│  ┌─────────────────┐  │               │  ┌─────────────────────────┐  │
│  │ PostgreSQL      │  │               │  │  BountyEscrow Smart    │  │
│  │ - profiles      │  │               │  │  Contract (PyTeal)     │  │
│  │ - bounties      │  │               │  │                         │  │
│  │ - submissions   │  │               │  │  - Global State        │  │
│  │ - disputes      │  │               │  │  - Local State         │  │
│  │ - txn_log       │  │               │  │  - ARC4 ABI Methods    │  │
│  └─────────────────┘  │               │  └─────────────────────────┘  │
│  ┌─────────────────┐  │               │                               │
│  │ Better Auth     │  │               │  ┌─────────────────────────┐  │
│  │ - user table    │  │               │  │  IPFS (Pinata)          │  │
│  │ - sessions      │  │               │  │                         │  │
│  └─────────────────┘  │               │  │  - Terms documents      │  │
└───────────────────────┘               │  │  - Work submissions      │  │
                                        │  │  - Dispute evidence     │  │
                                        │  └─────────────────────────┘  │
                                        └───────────────────────────────┘
```

---

## 4. Directory Structure

```
bounty-escrow/
│
├── backend/                          # Go REST API Server
│   ├── cmd/
│   │   └── main.go                   # Entry point, server setup, routing
│   │
│   ├── internal/
│   │   ├── config/
│   │   │   └── config.go             # Environment variable loading & validation
│   │   │
│   │   ├── handlers/
│   │   │   ├── auth.go               # Auth: register, login, refresh, profile
│   │   │   └── bounty.go             # Bounty CRUD, submissions, disputes
│   │   │
│   │   ├── middleware/
│   │   │   ├── auth.go               # JWT validation, role checking
│   │   │   └── security.go           # Rate limit, CORS, headers, recovery
│   │   │
│   │   ├── models/
│   │   │   └── models.go             # Domain models (Profile, Bounty, Submission, Dispute)
│   │   │
│   │   ├── services/
│   │   │   ├── algorand.go          # Blockchain interactions, txn building
│   │   │   └── ipfs.go              # Pinata IPFS integration
│   │   │
│   │   └── utils/
│   │       └── crypto.go            # SHA-256, bcrypt, Algorand address validation
│   │
│   ├── go.mod                        # Go module definition
│   └── go.sum                        # Dependency checksums
│
├── frontend/                         # Next.js 16 Application
│   ├── src/
│   │   └── app/
│   │       ├── page.tsx              # Home page component
│   │       ├── layout.tsx            # Root layout with providers
│   │       └── globals.css           # Global styles (Tailwind)
│   │
│   ├── public/                       # Static assets
│   │   ├── next.svg
│   │   ├── vercel.svg
│   │   └── window.svg
│   │
│   ├── database/
│   │   └── supabase-schema.sql      # PostgreSQL schema reference
│   │
│   ├── contracts/
│   │   ├── bounty_escrow.py          # Smart contract (PyTeal/Puya)
│   │   └── .algokit.toml            # AlgoKit configuration
│   │
│   ├── package.json                  # Frontend dependencies
│   ├── tsconfig.json                 # TypeScript configuration
│   ├── postcss.config.mjs            # PostCSS/Tailwind config
│   └── .next/                        # Build output (generated)
│
├── contracts/                       # Standalone smart contract files
│   ├── bounty_escrow.py             # PyTeal smart contract
│   └── .algokit.toml               # AlgoKit configuration
│
├── database/
│   └── supabase-schema.sql          # Complete PostgreSQL schema
│
├── flowchart.html                    # Visual architecture diagram
│
└── package.json                     # Root workspace scripts
```

---

## 5. System Flow & Process

### 5.1 User Registration & Authentication Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │────▶│   Backend   │────▶│  Supabase   │────▶│  Response   │
│  (Register) │     │  (Validate) │     │  (Store)    │     │  (JWT)      │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
                          │
                          ▼
                    ┌─────────────┐
                    │ bcrypt hash │
                    │  password   │
                    └─────────────┘
```

**Steps:**
1. Client sends `POST /api/auth/register` with email, password, username, role
2. Backend validates password strength (8+ chars, uppercase, lowercase, digit)
3. Password hashed with bcrypt (cost factor configurable, default 12)
4. UUID generated for auth user
5. User record inserted into Supabase `user` table
6. Profile record inserted into `profiles` table
7. JWT token generated with claims: `user_id`, `email`, `username`, `role`, `profile_id`
8. Refresh token generated (7-day TTL)
9. Response: `{ token, refresh_token, user }`

### 5.2 Bounty Creation Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Creator   │────▶│   Backend    │────▶│   Pinata     │────▶│ Supabase    │
│   Creates   │     │  (Process)   │     │   (IPFS)     │     │  (Store)    │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
                          │                     │
                          ▼                     ▼
                    ┌─────────────┐     ┌─────────────┐
                    │ SHA-256 Hash │     │  Terms CID  │
                    │ of CID      │     │  returned   │
                    └─────────────┘     └─────────────┘
```

**Steps:**
1. Creator calls `POST /api/bounties` with title, description, reward, deadline, tags
2. Backend validates deadline is in future (min 1 hour)
3. Minimum reward enforced: 1 ALGO
4. Bounty terms JSON pinned to IPFS via Pinata
5. SHA-256 hash computed from returned CID (32 bytes)
6. Bounty record created in Supabase with `status: "open"`
7. Terms CID and hash stored for on-chain verification
8. Response: bounty details + IPFS metadata + terms hash

### 5.3 Fund Locking Flow (On-Chain Activation)

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Creator    │────▶│   Backend    │────▶│  Algorand   │────▶│  Frontend   │
│   Requests   │     │  (Build Txn) │     │  (Prepare)  │     │  (Sign)     │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
                                                                      │
                          ┌──────────────────────────────────────────┘
                          ▼
                    ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
                    │  Signed     │────▶│   Backend    │────▶│  Algorand   │
                    │  Txn        │     │  (Submit)   │     │  (Confirm)  │
                    └─────────────┘     └─────────────┘     └─────────────┘
                                                                      │
                          ┌──────────────────────────────────────────┘
                          ▼
                    ┌─────────────┐
                    │  Escrow      │
                    │  Active      │
                    └─────────────┘
```

**Steps:**
1. Creator calls `POST /api/bounties/:id/lock`
2. Backend builds unsigned transaction group:
   - Payment transaction (creator → escrow address)
   - Application call transaction (`create_bounty` ABI method)
3. Frontend receives unsigned transactions (base64 encoded)
4. User signs transactions with wallet (Pera/AlgoSigner)
5. Signed transactions submitted to backend
6. Backend submits to Algorand Testnet
7. Wait for confirmation (polling `WaitForConfirmation`)
8. Update bounty record with `app_id` and `escrow_txn_id`
9. Log transaction in `transaction_log` table

### 5.4 Work Submission Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Worker    │────▶│   Backend    │────▶│   Pinata     │────▶│ Supabase    │
│   Submits   │     │  (Process)   │     │   (IPFS)     │     │  (Store)    │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
                          │                     │
                          ▼                     ▼
                    ┌─────────────┐     ┌─────────────┐
                    │ SHA-256 Hash │     │  Work CID   │
                    │ of CID      │     │  returned   │
                    └─────────────┘     └─────────────┘
                          │
                          ▼
                    ┌─────────────┐
                    │ Build On-   │
                    │ Chain Txn   │
                    └─────────────┘
```

**Steps:**
1. Worker uploads work file via `POST /api/bounties/:id/submit`
2. File validation: max 10MB (configurable)
3. File pinned to IPFS
4. SHA-256 hash computed from CID
5. Submission record created in Supabase
6. On-chain `submit_proof` transaction built
7. Worker signs and submits on-chain
8. Bounty status transitions: `open` → `in_progress`

### 5.5 Approval & Payout Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Creator   │────▶│   Backend    │────▶│  Algorand   │────▶│   Worker    │
│   Approves  │     │  (Build Txn) │     │  (Execute)  │     │  (Receive)  │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
                          │                     │
                          ▼                     ▼
                    ┌─────────────┐     ┌─────────────┐
                    │ Inner Txn   │     │ ALGO Trans. │
                    │ Prepared    │     │ to Worker   │
                    └─────────────┘     └─────────────┘
```

**Steps:**
1. Creator calls `PUT /api/bounties/:id/approve` with `submission_id`
2. Backend validates creator ownership
3. On-chain `approve_payout` transaction built
4. Creator signs and submits
5. Smart contract executes `InnerTransaction` payment to worker
6. Bounty status: `in_progress` → `completed`
7. Transaction logged in audit trail

### 5.6 Dispute Resolution Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Either    │────▶│   Backend    │────▶│   Pinata     │────▶│ Supabase    │
│   Party     │     │  (Record)   │     │   (Evidence)│     │  (Log)      │
│   Disputes  │     └─────────────┘     └─────────────┘     └─────────────┘
└─────────────┘
                          │
                          ▼
                    ┌─────────────┐     ┌─────────────┐
                    │ On-Chain    │────▶│  Funds      │
                    │ Dispute     │     │  Frozen     │
                    └─────────────┘     └─────────────┘
                          │
                          ▼
                    ┌─────────────┐
                    │  Creator    │
                    │  Resolves   │
                    └─────────────┘
                          │
              ┌───────────┴───────────┐
              ▼                       ▼
        ┌─────────────┐         ┌─────────────┐
        │ Refund to   │         │ Pay to      │
        │ Creator     │         │ Worker      │
        └─────────────┘         └─────────────┘
```

---

## 6. API Endpoints

### Authentication Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/auth/register` | None | Register new user |
| `POST` | `/api/auth/login` | None | Login user |
| `POST` | `/api/auth/refresh` | None | Refresh JWT token |
| `GET` | `/api/auth/me` | JWT | Get current user profile |
| `PUT` | `/api/auth/profile` | JWT | Update user profile |

### Bounty Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/bounties` | Optional | List all bounties (paginated) |
| `GET` | `/api/bounties/:id` | Optional | Get bounty details |
| `POST` | `/api/bounties` | JWT | Create new bounty |
| `POST` | `/api/bounties/:id/lock` | JWT | Build lock transaction |
| `POST` | `/api/bounties/:id/confirm-lock` | JWT | Confirm on-chain lock |
| `PUT` | `/api/bounties/:id/approve` | JWT | Approve submission |
| `PUT` | `/api/bounties/:id/reject` | JWT | Reject submission |
| `POST` | `/api/bounties/:id/submit` | JWT | Submit work |
| `POST` | `/api/bounties/:id/dispute` | JWT | Initiate dispute |
| `GET` | `/api/bounties/:id/submissions` | Optional | List submissions |

### IPFS Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/ipfs/pin` | JWT | Pin file to IPFS |

### Health Check

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/health` | None | Server health status |

---

## 7. Smart Contract Specification

### Contract: BountyEscrow

**Language:** PyTeal (Algorand Python / Puya)  
**Standard:** ARC4 ABI  
**Network:** Algorand Testnet

### Global State Variables

| Variable | Type | Description |
|----------|------|-------------|
| `creator` | Bytes (32) | Algorand address of bounty creator |
| `terms_hash` | Bytes (32) | SHA-256 hash of bounty terms IPFS CID |
| `reward` | UInt64 | Reward amount in microALGOs |
| `deadline` | UInt64 | Unix timestamp deadline |
| `status` | UInt64 | Bounty status enum (0-5) |
| `max_submissions` | UInt64 | Maximum allowed submissions |
| `submission_count` | UInt64 | Current submission count |
| `approved_worker` | Bytes (32) | Address of approved worker |

### Local State (per worker)

| Variable | Type | Description |
|----------|------|-------------|
| `work_hash` | Bytes (32) | SHA-256 hash of submitted work IPFS CID |
| `submission_time` | UInt64 | Timestamp of submission |
| `worker_status` | UInt64 | 0=none, 1=submitted, 2=approved, 3=rejected |

### Status Constants

```python
STATUS_OPEN = 0       # Bounty created, awaiting submissions
STATUS_IN_PROGRESS = 1 # At least one submission received
STATUS_COMPLETED = 2  # Payout completed
STATUS_DISPUTED = 3   # Dispute in progress
STATUS_EXPIRED = 4    # Deadline passed, refunded
STATUS_CANCELLED = 5  # Cancelled before submissions
```

### ABI Methods

#### `create_bounty(payment, terms_hash, deadline, max_submissions) → string`
- **Access:** Creator only
- **Args:**
  - `payment`: Grouped payment transaction to escrow
  - `terms_hash`: SHA-256 of IPFS CID (32 bytes)
  - `deadline`: Unix timestamp
  - `max_submissions`: 1-50 allowed
- **Security:** Payment receiver must be contract address; amount ≥ 1 ALGO

#### `opt_in() → string`
- **Access:** Workers only
- **Purpose:** Initialize local state for worker participation

#### `submit_proof(work_hash) → string`
- **Access:** Workers only
- **Args:** SHA-256 hash of work IPFS CID (32 bytes)
- **Security:** Cannot submit after deadline; max submissions enforced

#### `approve_payout(worker) → string`
- **Access:** Creator only
- **Effect:** InnerTransaction sends `reward` to worker
- **Security:** Worker must have submitted proof

#### `reject_submission(worker, reason) → string`
- **Access:** Creator only
- **Effect:** Worker status → rejected; submission count decremented

#### `initiate_dispute(evidence_hash) → string`
- **Access:** Creator or worker with submission
- **Effect:** Status → disputed; funds frozen

#### `resolve_dispute(in_favor_of) → string`
- **Access:** Creator only (MVP)
- **Effect:** Pays out to winner; status → completed

#### `refund_expired() → string`
- **Access:** Anyone
- **Effect:** Returns funds to creator after deadline

#### `cancel_bounty() → string`
- **Access:** Creator only
- **Condition:** Only if `submission_count == 0`

#### Read-Only Methods
- `get_bounty_info()` → (reward, deadline, status, submission_count, max_submissions)
- `get_worker_submission(worker)` → (work_hash, submission_time, worker_status)
- `get_escrow_balance()` → UInt64

---

## 8. Database Schema

### Entity-Relationship Diagram

```
┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
│    profiles     │       │    bounties     │       │  submissions    │
├─────────────────┤       ├─────────────────┤       ├─────────────────┤
│ id (PK)         │──┐    │ id (PK)         │──┐    │ id (PK)         │
│ auth_user_id    │  │    │ creator_id (FK) │◀─┘    │ bounty_id (FK)  │◀─┐
│ username        │  │    │ title           │       │ worker_id (FK)  │  │
│ display_name    │  │    │ description     │       │ work_ipfs_cid   │  │
│ wallet_address  │  │    │ reward_algo     │       │ work_hash       │  │
│ role            │  │    │ terms_ipfs_cid  │       │ status          │  │
│ reputation_score│  │    │ terms_hash      │       │ submitted_at    │  │
│ bio             │  │    │ deadline        │       │ reviewed_at     │  │
│ created_at      │  │    │ status          │       └─────────────────┘  │
└─────────────────┘  │    │ app_id          │               │         │
       │              │    │ escrow_txn_id   │               │         │
       │              │    └─────────────────┘               │         │
       │              │                                    │         │
       │              │    ┌─────────────────┐              │         │
       │              │    │    disputes     │              │         │
       │              │    ├─────────────────┤              │         │
       │              │    │ id (PK)         │              │         │
       │              │    │ bounty_id (FK)  │◀─────────────┘         │
       │              │    │ submission_id   │◀─────────────────────────┘
       │              │    │ initiated_by    │
       │              │    │ reason          │
       │              │    │ status          │
       │              │    │ resolution_notes│
       │              │    └─────────────────┘
       │              │
       │              │    ┌─────────────────┐
       │              │    │ transaction_log │
       │              │    ├─────────────────┤
       │              │    │ id (PK)         │
       └──────────────│───▶│ bounty_id (FK)  │
                      │    │ actor_id (FK)   │
                      │    │ action          │
                      │    │ txn_id          │
                      │    │ amount_algo     │
                      │    │ metadata (JSONB) │
                      │    │ created_at      │
                      │    └─────────────────┘
                      │
                      └──────────────────▶┌─────────────────┐
                                           │  user (Better)  │
                                           ├─────────────────┤
                                           │ id (PK)        │
                                           │ email          │
                                           │ password_hash  │
                                           │ email_verified │
                                           │ created_at     │
                                           └─────────────────┘
```

### Tables

#### `profiles`
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK, default gen_random_uuid() |
| auth_user_id | TEXT | UNIQUE, NOT NULL |
| username | VARCHAR(100) | UNIQUE, NOT NULL |
| display_name | VARCHAR(200) | |
| avatar_url | TEXT | Cloudflare R2 URL |
| wallet_address | VARCHAR(58) | Algorand address |
| role | user_role | DEFAULT 'worker' |
| bio | TEXT | |
| reputation_score | INT | DEFAULT 0 |
| total_bounties_created | INT | DEFAULT 0 |
| total_bounties_completed | INT | DEFAULT 0 |

#### `bounties`
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK |
| creator_id | UUID | FK → profiles(id) |
| title | VARCHAR(300) | NOT NULL |
| description | TEXT | NOT NULL |
| reward_algo | DECIMAL(18,6) | CHECK > 0 |
| terms_ipfs_cid | VARCHAR(100) | Pinata CID |
| terms_hash | BYTEA | SHA-256 (32 bytes) |
| deadline | TIMESTAMPTZ | CHECK > NOW() |
| status | bounty_status | DEFAULT 'open' |
| app_id | BIGINT | Algorand app ID |
| escrow_txn_id | VARCHAR(52) | Algorand txn ID |
| max_submissions | INT | DEFAULT 5, CHECK > 0 |
| tags | TEXT[] | |

#### `submissions`
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK |
| bounty_id | UUID | FK → bounties(id) |
| worker_id | UUID | FK → profiles(id) |
| work_ipfs_cid | VARCHAR(100) | NOT NULL |
| work_hash | BYTEA | NOT NULL (32 bytes) |
| description | TEXT | |
| submission_txn_id | VARCHAR(52) | |
| status | submission_status | DEFAULT 'pending' |
| feedback | TEXT | |

#### `disputes`
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK |
| bounty_id | UUID | FK → bounties(id) |
| submission_id | UUID | FK → submissions(id) |
| initiated_by | UUID | FK → profiles(id) |
| reason | TEXT | NOT NULL |
| evidence_ipfs_cid | VARCHAR(100) | |
| status | dispute_status | DEFAULT 'open' |
| resolution_notes | TEXT | |
| resolved_at | TIMESTAMPTZ | |

#### `transaction_log` (Immutable Audit Trail)
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK |
| bounty_id | UUID | FK → bounties(id) |
| actor_id | UUID | FK → profiles(id) |
| action | VARCHAR(50) | NOT NULL |
| txn_id | VARCHAR(52) | |
| amount_algo | DECIMAL(18,6) | |
| metadata | JSONB | |
| created_at | TIMESTAMPTZ | |

### Enums

```sql
CREATE TYPE user_role AS ENUM ('creator', 'worker', 'admin');
CREATE TYPE bounty_status AS ENUM ('open', 'in_progress', 'submitted', 'completed', 'disputed', 'expired', 'cancelled');
CREATE TYPE submission_status AS ENUM ('pending', 'approved', 'rejected', 'disputed');
CREATE TYPE dispute_status AS ENUM ('open', 'resolved_creator', 'resolved_worker', 'escalated');
```

---

## 9. Security Architecture

### 9.1 Authentication & Authorization

#### JWT Token Structure
```json
{
  "user_id": "uuid",
  "email": "user@example.com",
  "username": "johndoe",
  "role": "creator",
  "profile_id": "uuid",
  "exp": 1712000000,
  "iat": 1711913600,
  "iss": "bountyvault-api",
  "sub": "user_uuid"
}
```

#### Token Configuration
| Setting | Value |
|---------|-------|
| Algorithm | HS256 (HMAC-SHA256) |
| Access Token TTL | 24 hours |
| Refresh Token TTL | 7 days |
| Clock Skew Tolerance | 5 seconds |
| Minimum Secret Length | 32 characters |

### 9.2 Middleware Stack (Order of Execution)

```
1. RecoveryMiddleware     → Panic recovery (crash protection)
2. RequestIDMiddleware    → Request tracing (UUID generation)
3. RequestLoggerMiddleware → Request/response logging
4. SecurityHeadersMiddleware → HTTP security headers
5. RateLimitMiddleware    → Per-IP rate limiting
6. MaxBodySizeMiddleware  → Request body limits
7. CORS Configuration     → Cross-origin requests
8. AuthMiddleware         → JWT validation
```

### 9.3 Security Headers

```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
Referrer-Policy: strict-origin-when-cross-origin
Content-Security-Policy: default-src 'none'; frame-ancestors 'none'
Permissions-Policy: camera=(), microphone=(), geolocation=(), payment=()
Cache-Control: no-store, no-cache, must-revalidate, private
```

### 9.4 Rate Limiting

| Endpoint | RPS | Burst |
|----------|-----|-------|
| General API | 10 | 20 |
| Auth endpoints | 5 | 10 |
| File uploads | 2 | 4 |

### 9.5 Password Security

- Minimum length: 8 characters
- Required: uppercase, lowercase, digit
- Bcrypt cost factor: 12 (configurable)
- Timing-safe comparison for verification

### 9.6 Input Validation

- All inputs sanitized and trimmed
- UUID validation on path parameters
- RFC3339 date format validation
- Algorand address format validation (58 chars, base32)
- File size limits enforced (default 10MB)

---

## 10. Implementation Workflow

### Development Setup

```bash
# 1. Clone and install dependencies
npm run install:all

# 2. Configure environment
cp backend/.env.example backend/.env
# Edit backend/.env with your API keys

# 3. Start development servers
npm run dev:frontend    # Next.js on :3000
npm run dev:backend     # Go API on :8080
```

### Build Process

```bash
# Build both frontend and backend
npm run build

# Individual builds
npm run build:frontend  # Next.js production build
npm run build:backend   # Go binary compilation
```

### Environment Variables

#### Backend (.env)
```bash
# Server
PORT=8080
ENVIRONMENT=development
ALLOW_ORIGINS=http://localhost:3000

# JWT
JWT_SECRET=your-32-char-minimum-secret-key
JWT_EXPIRY=24h
JWT_REFRESH_TTL=168h

# Supabase
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_SERVICE_ROLE_KEY=your-service-role-key
SUPABASE_ANON_KEY=your-anon-key

# Better Auth
BETTER_AUTH_SECRET=your-32-char-secret

# Algorand
ALGO_NODE_URL=https://testnet-api.4160.nodely.dev
ALGO_NODE_TOKEN=
ALGO_INDEXER_URL=https://testnet-idx.4160.nodely.dev
ALGO_NETWORK=testnet

# Pinata
PINATA_JWT=your-pinata-jwt-token

# Cloudflare R2 (optional)
R2_ACCOUNT_ID=
R2_ACCESS_KEY=
R2_SECRET_KEY=

# Security
BCRYPT_COST=12
MAX_UPLOAD_SIZE_MB=10
RATE_LIMIT_RPS=10
RATE_LIMIT_BURST=20
```

### Smart Contract Deployment

```bash
# Using AlgoKit
algokit deploy

# Or manual deployment
# 1. Compile PyTeal to TEAL
# 2. Create application on-chain
# 3. Note the App ID for configuration
```

### Database Migration

```bash
# Apply schema to Supabase
# Use Supabase dashboard or CLI
supabase db push

# Or execute SQL directly
psql -h your-host -U postgres -d postgres -f database/supabase-schema.sql
```

---

## Appendix: API Response Format

### Success Response
```json
{
  "success": true,
  "message": "Operation completed successfully",
  "data": { ... }
}
```

### Error Response
```json
{
  "success": false,
  "error": "Error description"
}
```

### Paginated Response
```json
{
  "success": true,
  "data": {
    "items": [ ... ],
    "total_count": 100,
    "page": 1,
    "page_size": 20,
    "total_pages": 5
  }
}
```

---

*Document Version: 1.0.0*  
*Last Updated: April 2026*
