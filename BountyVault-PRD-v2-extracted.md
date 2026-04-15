__BountyVault__

Decentralized Bounty Escrow System on Algorand

Product Requirements Document  |  v2\.0  |  April 2026

__AlgoBharat Hackathon Submission__

__What changed in v2\.0__

1\. Dispute resolution upgraded: creator\-only dispute control replaced with arbitrator role \(trustless fix\)
2\. Auto\-refund trigger added for expired bounties
3\. Reputation score logic wired to bounty completion
4\. Search/filter API added for bounty discovery
5\. Multi\-submission leaderboard view added
6\. On\-chain note field for Algorand transactions
7\. Next\.js version corrected to 15\.x
8\. contracts/ directory consolidated to root level
9\. Public bounty explorer \(no\-auth view\) added

# __1\. Project Overview__

## __1\.1 Project Name__

BountyVault ΓÇö Decentralized Bounty Escrow System

## __1\.2 Core Functionality__

A trustless bounty platform built on Algorand blockchain that enables:

- Creators to post bounties with locked ALGO rewards
- Workers to submit work proofs via IPFS\-hashed submissions
- Automatic escrow payouts through smart contracts
- Decentralized dispute resolution with a neutral arbitrator role
- Public bounty explorer for discoverability without authentication

## __1\.3 Key Value Propositions__

__Property__

__Detail__

Trustless

No intermediary for fund custody ΓÇö smart contracts handle escrow; disputes routed to arbitrator, not just creator

Transparent

All transactions immutable and auditable on\-chain; on\-chain note fields human\-readable in block explorer

Fast

Algorand's 3\.3s block finality enables near\-instant payout confirmation

Cost\-effective

Transaction fees < $0\.001 per operation

Reputation

Worker reputation score updates on\-chain on every completed bounty

# __2\. Tech Stack__

## __2\.1 Frontend__

__Technology__

__Version__

__Purpose__

Next\.js

15\.x

React framework with SSR/SSG

React

19\.x

UI library

TypeScript

5\.x

Type safety

Tailwind CSS

4\.x

Styling

## __2\.2 Backend__

__Technology__

__Version__

__Purpose__

Go

Latest

REST API server

Gin

Latest

HTTP web framework

go\-algorand\-sdk

v2

Algorand blockchain interaction

## __2\.3 Blockchain & Storage__

__Technology__

__Network__

__Purpose__

Algorand

Testnet

Smart contract escrow

PyTeal \(Puya\)

AVM v10

Smart contract language

AlgoKit

Latest

Algorand development toolkit

IPFS \(Pinata\)

Cloud

Decentralized file storage

Supabase

PostgreSQL

Relational database \+ auth

## __2\.4 Infrastructure__

__Technology__

__Purpose__

Better Auth

Authentication framework

Cloudflare R2

Avatar/media storage

JWT \(HS256\)

Session management

# __3\. Architecture Overview__

The system is a three\-tier architecture: Next\.js frontend ΓåÆ Go/Gin REST API ΓåÆ Algorand blockchain \+ Supabase DB\. IPFS \(Pinata\) sits alongside as the decentralized file store\.

CLIENT LAYER  \(Next\.js 15\)

  Auth  |  Bounty List  |  Work Submission  |  Wallet Connect \(Pera\)

             |  HTTPS/REST

API LAYER  \(Go / Gin\)

  Middleware: Recovery ΓåÆ RequestID ΓåÆ Logger ΓåÆ Security ΓåÆ RateLimit

  AuthHandler  |  BountyHandler  |  AlgorandService  |  IPFSService

        |                                    |

   Supabase                        Algorand Testnet

  \(PostgreSQL\)                   BountyEscrow Smart Contract

  profiles / bounties                  \(ARC4 ABI / PyTeal\)

  submissions / disputes                      \+

  txn\_log                           IPFS \(Pinata\)

# __4\. Directory Structure__

bounty\-escrow/

Γö£ΓöÇΓöÇ backend/                     \# Go REST API Server

Γöé   Γö£ΓöÇΓöÇ cmd/main\.go               \# Entry point, routing

Γöé   ΓööΓöÇΓöÇ internal/

Γöé       Γö£ΓöÇΓöÇ config/config\.go      \# Env var loading

Γöé       Γö£ΓöÇΓöÇ handlers/

Γöé       Γöé   Γö£ΓöÇΓöÇ auth\.go           \# Register, login, profile

Γöé       Γöé   ΓööΓöÇΓöÇ bounty\.go         \# Bounty CRUD, submit, dispute

Γöé       Γö£ΓöÇΓöÇ middleware/

Γöé       Γöé   Γö£ΓöÇΓöÇ auth\.go           \# JWT validation

Γöé       Γöé   ΓööΓöÇΓöÇ security\.go       \# Rate limit, CORS, headers

Γöé       Γö£ΓöÇΓöÇ models/models\.go      \# Domain models

Γöé       ΓööΓöÇΓöÇ services/

Γöé           Γö£ΓöÇΓöÇ algorand\.go       \# Blockchain interactions

Γöé           ΓööΓöÇΓöÇ ipfs\.go           \# Pinata IPFS integration

Γö£ΓöÇΓöÇ frontend/                     \# Next\.js 15 Application

Γöé   ΓööΓöÇΓöÇ src/app/

Γöé       Γö£ΓöÇΓöÇ page\.tsx              \# Home / bounty explorer

Γöé       Γö£ΓöÇΓöÇ layout\.tsx            \# Root layout

Γöé       ΓööΓöÇΓöÇ globals\.css           \# Tailwind globals

Γö£ΓöÇΓöÇ contracts/                    \# Consolidated smart contracts

Γöé   Γö£ΓöÇΓöÇ bounty\_escrow\.py          \# PyTeal/Puya contract

Γöé   ΓööΓöÇΓöÇ \.algokit\.toml             \# AlgoKit config

ΓööΓöÇΓöÇ database/

    ΓööΓöÇΓöÇ supabase\-schema\.sql       \# PostgreSQL schema

# __5\. System Flows__

## __5\.1 User Registration & Authentication__

1\. Client sends POST /api/auth/register with email, password, username, role

2\. Backend validates password strength \(8\+ chars, uppercase, lowercase, digit\)

3\. Password hashed with bcrypt \(cost factor 12\)

4\. UUID generated; user \+ profile records inserted into Supabase

5\. JWT \(24h\) and refresh token \(7d\) returned

## __5\.2 Bounty Creation__

1\. Creator calls POST /api/bounties with title, description, reward, deadline, tags

2\. Backend enforces: deadline > now \+ 1hr, reward >= 1 ALGO

3\. Bounty terms JSON pinned to IPFS via Pinata ΓåÆ SHA\-256 hash computed

4\. Bounty record created in Supabase with status: open

5\. Response includes bounty details, IPFS CID, and terms hash

## __5\.3 Fund Locking \(On\-Chain Activation\)__

1\. Creator calls POST /api/bounties/:id/lock

2\. Backend builds unsigned transaction group: \[Payment to escrow\] \+ \[create\_bounty app call\]

3\. Frontend receives base64\-encoded unsigned transactions

4\. User signs with Pera Wallet

5\. Signed transactions submitted to backend, then to Algorand Testnet

6\. On confirmation: bounty record updated with app\_id and escrow\_txn\_id

7\. Transaction note field set to: BountyVault:create:\{bounty\_uuid\} for block explorer readability

## __5\.4 Work Submission__

1\. Worker uploads file via POST /api/bounties/:id/submit \(max 10MB\)

2\. File pinned to IPFS; SHA\-256 hash computed from CID

3\. Submission record created in Supabase

4\. On\-chain submit\_proof transaction built and signed by worker

5\. Bounty status transitions: open ΓåÆ in\_progress

## __5\.5 Approval & Payout__

1\. Creator calls PUT /api/bounties/:id/approve with submission\_id

2\. Backend validates creator ownership

3\. approve\_payout transaction built; creator signs

4\. Smart contract executes InnerTransaction to pay worker

5\. Bounty status: in\_progress ΓåÆ completed

6\. Worker reputation\_score incremented in Supabase

7\. Transaction note: BountyVault:payout:\{bounty\_uuid\}

## __5\.6 Dispute Resolution \(Updated ΓÇö Arbitrator Model\)__

__v2\.0 Change ΓÇö Why This Matters__

In v1\.0, only the creator could resolve disputes, creating a trust dependency that contradicts the platform's trustless goal\. v2\.0 introduces an arbitrator address set at contract creation\. Disputes escalate to the arbitrator, who can pay the worker or refund the creator\. The creator cannot unilaterally resolve a dispute\.

1\. Either party calls POST /api/bounties/:id/dispute with reason and evidence file

2\. Evidence file pinned to IPFS; hash stored on\-chain via initiate\_dispute

3\. Bounty status ΓåÆ disputed; funds frozen in escrow

4\. Arbitrator \(set at contract creation\) reviews evidence off\-chain

5\. Arbitrator calls resolve\_dispute\(in\_favor\_of\) on\-chain

6\. Smart contract pays winner; status ΓåÆ completed or refunded

7\. If arbitrator does not act within 30 days: auto\-refund to creator via refund\_expired

## __5\.7 Auto\-Refund \(Expired Bounty\)  \[NEW\]__

1\. Any user \(creator, worker, or public\) can call POST /api/bounties/:id/refund\-expired

2\. Backend calls the refund\_expired ABI method on\-chain

3\. Smart contract checks: Global\.latest\_timestamp > deadline

4\. If expired: InnerTransaction refunds creator; status ΓåÆ expired

5\. Frontend shows a Trigger Refund button when deadline has passed

# __6\. API Endpoints__

## __6\.1 Authentication__

__Method__

__Endpoint__

__Auth__

__Description__

POST

/api/auth/register

None

Register new user

POST

/api/auth/login

None

Login user

POST

/api/auth/refresh

None

Refresh JWT token

GET

/api/auth/me

JWT

Get current user profile

PUT

/api/auth/profile

JWT

Update user profile

## __6\.2 Bounty Endpoints__

__Method__

__Endpoint__

__Auth__

__Description__

GET

/api/bounties

Optional

List bounties \(paginated, filterable\)

GET

/api/bounties/:id

Optional

Get bounty details \+ submissions

POST

/api/bounties

JWT

Create new bounty

POST

/api/bounties/:id/lock

JWT

Build fund\-lock transaction

POST

/api/bounties/:id/confirm\-lock

JWT

Confirm on\-chain fund lock

PUT

/api/bounties/:id/approve

JWT

Approve submission \+ trigger payout

PUT

/api/bounties/:id/reject

JWT

Reject submission with feedback

POST

/api/bounties/:id/submit

JWT

Submit work proof

POST

/api/bounties/:id/dispute

JWT

Initiate dispute

POST

/api/bounties/:id/refund\-expired

None

Trigger auto\-refund if expired  \[NEW\]

GET

/api/bounties/:id/submissions

Optional

List submissions \(leaderboard view\)  \[NEW\]

## __6\.3 Filter Parameters for GET /api/bounties  \[NEW\]__

__Param__

__Type__

__Example__

__Description__

tag

string

?tag=frontend

Filter by tag

status

enum

?status=open

Filter by bounty status

min\_reward

float

?min\_reward=5

Minimum reward in ALGO

max\_reward

float

?max\_reward=100

Maximum reward in ALGO

sort

enum

?sort=reward\_desc

Sort: reward\_asc, reward\_desc, deadline\_asc

## __6\.4 IPFS & Health__

__Method__

__Endpoint__

__Auth__

__Description__

POST

/api/ipfs/pin

JWT

Pin file to IPFS via Pinata

GET

/health

None

Server health \+ Algorand node status

# __7\. Smart Contract Specification__

Language: PyTeal \(Algorand Python / Puya\)  |  Standard: ARC4 ABI  |  Network: Algorand Testnet

## __7\.1 Global State Variables__

__Variable__

__Type__

__Description__

creator

Bytes \(32\)

Algorand address of bounty creator

arbitrator

Bytes \(32\)

Neutral arbitrator address ΓÇö set at contract creation  \[NEW\]

terms\_hash

Bytes \(32\)

SHA\-256 hash of bounty terms IPFS CID

reward

UInt64

Reward amount in microALGOs

deadline

UInt64

Unix timestamp deadline

dispute\_deadline

UInt64

Deadline for arbitrator to act \(deadline \+ 30 days\)  \[NEW\]

status

UInt64

Bounty status enum \(0\-6\)

max\_submissions

UInt64

Maximum allowed submissions \(1\-50\)

submission\_count

UInt64

Current submission count

approved\_worker

Bytes \(32\)

Address of approved/winning worker

## __7\.2 Local State \(per worker\)__

__Variable__

__Type__

__Description__

work\_hash

Bytes \(32\)

SHA\-256 hash of submitted work IPFS CID

submission\_time

UInt64

Unix timestamp of submission

worker\_status

UInt64

0=none, 1=submitted, 2=approved, 3=rejected

## __7\.3 Status Constants__

STATUS\_OPEN         = 0   \# Bounty created, awaiting submissions

STATUS\_IN\_PROGRESS  = 1   \# At least one submission received

STATUS\_COMPLETED    = 2   \# Payout completed

STATUS\_DISPUTED     = 3   \# Dispute in progress ΓÇö funds frozen

STATUS\_EXPIRED      = 4   \# Deadline passed, refunded

STATUS\_CANCELLED    = 5   \# Cancelled before any submissions

STATUS\_ARBITRATING  = 6   \# Dispute sent to arbitrator \[NEW\]

## __7\.4 ABI Methods__

### __create\_bounty\(payment, terms\_hash, deadline, max\_submissions, arbitrator\) ΓåÆ string  \[UPDATED\]__

- Access: Anyone \(caller becomes creator\)
- Args: payment \(grouped txn to escrow\), terms\_hash \(32 bytes\), deadline \(unix ts\), max\_submissions \(1\-50\), arbitrator \(Algorand address\)
- Security: Payment receiver must be contract address; amount >= 1 ALGO; sets arbitrator in global state

### __opt\_in\(\) ΓåÆ string__

- Access: Workers only
- Purpose: Initialize local state for worker participation

### __submit\_proof\(work\_hash\) ΓåÆ string__

- Access: Workers only
- Args: SHA\-256 hash of work IPFS CID \(32 bytes\)
- Security: Cannot submit after deadline; max\_submissions enforced; one submission per worker

### __approve\_payout\(worker\) ΓåÆ string__

- Access: Creator only
- Effect: InnerTransaction sends reward to worker; reputation\_score update noted in txn note
- Security: Worker must have submitted proof; status must be IN\_PROGRESS

### __reject\_submission\(worker, reason\) ΓåÆ string__

- Access: Creator only
- Effect: Worker status ΓåÆ rejected; submission\_count decremented

### __initiate\_dispute\(evidence\_hash\) ΓåÆ string__

- Access: Creator or worker with a valid submission
- Effect: Status ΓåÆ DISPUTED; funds frozen; dispute\_deadline set to now \+ 30 days

### __resolve\_dispute\(in\_favor\_of\) ΓåÆ string  \[UPDATED ΓÇö Arbitrator Only\]__

__Critical Change from v1\.0__

Access restricted to arbitrator address only \(NOT creator\)\. This prevents the party who may be acting in bad faith from controlling the resolution outcome\.

- Access: Arbitrator only
- Args: in\_favor\_of \(Algorand address ΓÇö either creator or approved worker\)
- Effect: InnerTransaction pays winner; status ΓåÆ COMPLETED

### __refund\_expired\(\) ΓåÆ string  \[NEW\]__

- Access: Anyone
- Condition: Global\.latest\_timestamp > deadline OR \(status == DISPUTED AND now > dispute\_deadline\)
- Effect: InnerTransaction refunds creator; status ΓåÆ EXPIRED

### __cancel\_bounty\(\) ΓåÆ string__

- Access: Creator only
- Condition: submission\_count == 0
- Effect: Refunds creator; status ΓåÆ CANCELLED

### __Read\-Only Methods__

- get\_bounty\_info\(\) ΓåÆ \(reward, deadline, status, submission\_count, max\_submissions, arbitrator\)
- get\_worker\_submission\(worker\) ΓåÆ \(work\_hash, submission\_time, worker\_status\)
- get\_escrow\_balance\(\) ΓåÆ UInt64

# __8\. Database Schema__

## __8\.1 Enums__

CREATE TYPE user\_role AS ENUM \('creator', 'worker', 'admin', 'arbitrator'\);

CREATE TYPE bounty\_status AS ENUM \(

  'open', 'in\_progress', 'submitted', 'completed',

  'disputed', 'arbitrating', 'expired', 'cancelled'

\);

CREATE TYPE submission\_status AS ENUM \('pending','approved','rejected','disputed'\);

CREATE TYPE dispute\_status AS ENUM \(

  'open', 'resolved\_creator', 'resolved\_worker', 'escalated', 'auto\_refunded'

\);

## __8\.2 profiles__

__Column__

__Type__

__Constraints / Notes__

id

UUID

PK, default gen\_random\_uuid\(\)

auth\_user\_id

TEXT

UNIQUE, NOT NULL

username

VARCHAR\(100\)

UNIQUE, NOT NULL

wallet\_address

VARCHAR\(58\)

Algorand address \(58 chars\)

role

user\_role

DEFAULT 'worker'

reputation\_score

INT

DEFAULT 0 ΓÇö incremented on bounty completion  \[WIRED\]

total\_bounties\_created

INT

DEFAULT 0

total\_bounties\_completed

INT

DEFAULT 0

bio

TEXT

avatar\_url

TEXT

Cloudflare R2 URL

## __8\.3 bounties__

__Column__

__Type__

__Constraints / Notes__

id

UUID

PK

creator\_id

UUID

FK ΓåÆ profiles\(id\)

title

VARCHAR\(300\)

NOT NULL

description

TEXT

NOT NULL

reward\_algo

DECIMAL\(18,6\)

CHECK > 0

terms\_ipfs\_cid

VARCHAR\(100\)

Pinata CID

terms\_hash

BYTEA

SHA\-256 \(32 bytes\)

deadline

TIMESTAMPTZ

CHECK > NOW\(\)

status

bounty\_status

DEFAULT 'open'

app\_id

BIGINT

Algorand app ID

escrow\_txn\_id

VARCHAR\(52\)

Algorand txn ID

arbitrator\_address

VARCHAR\(58\)

Algorand address of arbitrator  \[NEW\]

max\_submissions

INT

DEFAULT 5, CHECK > 0

tags

TEXT\[\]

Array of tag strings

## __8\.4 submissions__

__Column__

__Type__

__Constraints / Notes__

id

UUID

PK

bounty\_id

UUID

FK ΓåÆ bounties\(id\)

worker\_id

UUID

FK ΓåÆ profiles\(id\)

work\_ipfs\_cid

VARCHAR\(100\)

NOT NULL

work\_hash

BYTEA

NOT NULL \(32 bytes\)

description

TEXT

submission\_txn\_id

VARCHAR\(52\)

Algorand txn ID

status

submission\_status

DEFAULT 'pending'

feedback

TEXT

Creator feedback on rejection

## __8\.5 disputes__

__Column__

__Type__

__Constraints / Notes__

id

UUID

PK

bounty\_id

UUID

FK ΓåÆ bounties\(id\)

submission\_id

UUID

FK ΓåÆ submissions\(id\)

initiated\_by

UUID

FK ΓåÆ profiles\(id\)

reason

TEXT

NOT NULL

evidence\_ipfs\_cid

VARCHAR\(100\)

status

dispute\_status

DEFAULT 'open'

arbitrator\_address

VARCHAR\(58\)

Address of arbitrator for this dispute  \[NEW\]

resolution\_notes

TEXT

resolved\_at

TIMESTAMPTZ

auto\_refund\_after

TIMESTAMPTZ

Arbitrator deadline ΓÇö 30 days from dispute creation  \[NEW\]

## __8\.6 transaction\_log__

__Column__

__Type__

__Constraints / Notes__

id

UUID

PK

bounty\_id

UUID

FK ΓåÆ bounties\(id\)

actor\_id

UUID

FK ΓåÆ profiles\(id\)

action

VARCHAR\(50\)

NOT NULL

txn\_id

VARCHAR\(52\)

Algorand txn ID

txn\_note

VARCHAR\(200\)

On\-chain note field value  \[NEW\]

amount\_algo

DECIMAL\(18,6\)

metadata

JSONB

created\_at

TIMESTAMPTZ

# __9\. Security Architecture__

## __9\.1 JWT Token Configuration__

__Setting__

__Value__

Algorithm

HS256 \(HMAC\-SHA256\)

Access Token TTL

24 hours

Refresh Token TTL

7 days

Clock Skew Tolerance

5 seconds

Minimum Secret Length

32 characters

## __9\.2 Middleware Stack__

Order of execution:

1\. RecoveryMiddleware ΓåÆ panic recovery

2\. RequestIDMiddleware ΓåÆ UUID\-based request tracing

3\. RequestLoggerMiddleware ΓåÆ structured logging

4\. SecurityHeadersMiddleware ΓåÆ HTTP security headers

5\. RateLimitMiddleware ΓåÆ per\-IP rate limiting

6\. MaxBodySizeMiddleware ΓåÆ 10MB default limit

7\. CORS Configuration ΓåÆ cross\-origin control

8\. AuthMiddleware ΓåÆ JWT validation

## __9\.3 Rate Limiting__

__Endpoint__

__RPS__

__Burst__

General API

10

20

Auth endpoints

5

10

File uploads

2

4

## __9\.4 Security Headers__

X\-Content\-Type\-Options: nosniff

X\-Frame\-Options: DENY

X\-XSS\-Protection: 1; mode=block

Strict\-Transport\-Security: max\-age=31536000; includeSubDomains

Referrer\-Policy: strict\-origin\-when\-cross\-origin

Content\-Security\-Policy: default\-src 'none'; frame\-ancestors 'none'

Cache\-Control: no\-store, no\-cache, must\-revalidate, private

# __10\. New Features \(v2\.0\)__

## __10\.1 Reputation Score System__

__Wired in v2\.0__

Previously, the reputation\_score column existed in the DB but was never updated\. v2\.0 wires it to the bounty completion flow\.

- On approve\_payout: reputation\_score \+= 1 for approved worker
- On cancel by worker \(future\): reputation\_score \-= 1 \(penalise abandonment\)
- Displayed on worker profile cards and submission leaderboard

## __10\.2 Multi\-Submission Leaderboard__

- GET /api/bounties/:id/submissions returns all submissions sorted by submission\_time
- Frontend renders a table: worker username, reputation score, submission time, IPFS link, status
- Creator can approve any submission from the leaderboard view
- Supports max\_submissions up to 50 ΓÇö demonstrates platform scalability

## __10\.3 On\-Chain Transaction Notes__

- Every Algorand transaction built by the backend includes a structured note field
- Format: BountyVault:\{action\}:\{bounty\_uuid\}
- Actions: create, lock, submit, approve, dispute, resolve, refund, cancel
- Notes are readable in any Algorand block explorer \(e\.g\., AlgoExplorer, Lora\)
- Stored in transaction\_log\.txn\_note for off\-chain auditability

## __10\.4 Public Bounty Explorer__

- GET /api/bounties requires no authentication
- Frontend home page renders all open bounties as cards ΓÇö title, reward, deadline, tags, submission count
- Filterable by tag, status, reward range \(see Section 6\.3\)
- Unauthenticated users can browse and view bounty details
- Connect Wallet CTA shown for actions requiring auth \(submit, create\)

## __10\.5 Expired Bounty Refund Trigger__

- Any user \(including public\) can trigger refund\_expired if deadline has passed
- Frontend displays a Trigger Refund button on bounty detail page when deadline < now
- Smart contract validates deadline on\-chain ΓÇö no backend trust required
- Also auto\-triggers if dispute deadline exceeded \(30 days with no arbitrator action\)

# __11\. Implementation Workflow__

## __11\.1 Development Setup__

\# 1\. Clone and install dependencies

npm run install:all

\# 2\. Configure environment

cp backend/\.env\.example backend/\.env

\# 3\. Start development servers

npm run dev:frontend    \# Next\.js on :3000

npm run dev:backend     \# Go API on :8080

\# 4\. Deploy smart contract

cd contracts && algokit deploy

## __11\.2 Environment Variables__

\# Server

PORT=8080

ENVIRONMENT=development

ALLOW\_ORIGINS=http://localhost:3000

\# JWT

JWT\_SECRET=your\-32\-char\-minimum\-secret\-key

JWT\_EXPIRY=24h

JWT\_REFRESH\_TTL=168h

\# Supabase

SUPABASE\_URL=https://your\-project\.supabase\.co

SUPABASE\_SERVICE\_ROLE\_KEY=your\-service\-role\-key

\# Algorand

ALGO\_NODE\_URL=https://testnet\-api\.4160\.nodely\.dev

ALGO\_INDEXER\_URL=https://testnet\-idx\.4160\.nodely\.dev

ALGO\_NETWORK=testnet

ALGO\_ARBITRATOR\_ADDRESS=your\-arbitrator\-wallet\-address  \# NEW

\# Pinata

PINATA\_JWT=your\-pinata\-jwt\-token

\# Security

BCRYPT\_COST=12

MAX\_UPLOAD\_SIZE\_MB=10

RATE\_LIMIT\_RPS=10

RATE\_LIMIT\_BURST=20

# __12\. Super\-Detailed Implementation Prompt__

Copy the section below as your AI coding assistant prompt\. It includes the full context, constraints, and all v2\.0 changes\.

__SYSTEM CONTEXT__

You are a senior full\-stack blockchain developer building BountyVault, a decentralized bounty escrow system for the AlgoBharat Hackathon\. You have deep expertise in Go, Next\.js, Algorand smart contracts \(PyTeal/Puya\), and IPFS\. You always write production\-quality code with proper error handling, security headers, and on\-chain validations\.

__TECH STACK__

Backend: Go \+ Gin framework \+ go\-algorand\-sdk v2

Frontend: Next\.js 15 \+ React 19 \+ TypeScript \+ Tailwind CSS 4

Smart Contract: PyTeal \(Puya\) on Algorand Testnet, ARC4 ABI standard

Storage: Supabase \(PostgreSQL\) \+ IPFS via Pinata \+ Cloudflare R2

__CORE REQUIREMENTS__

__1\. SMART CONTRACT \(contracts/bounty\_escrow\.py\)__

Write an ARC4\-compliant PyTeal smart contract with these global state variables: creator \(bytes32\), arbitrator \(bytes32\), terms\_hash \(bytes32\), reward \(uint64, microALGO\), deadline \(uint64, unix timestamp\), dispute\_deadline \(uint64, deadline \+ 2592000 = 30 days\), status \(uint64\), max\_submissions \(uint64, 1\-50\), submission\_count \(uint64\), approved\_worker \(bytes32\)\.

Local state per worker: work\_hash \(bytes32\), submission\_time \(uint64\), worker\_status \(uint64: 0=none,1=submitted,2=approved,3=rejected\)\.

Implement ABI methods: create\_bounty\(payment, terms\_hash, deadline, max\_submissions, arbitrator\), opt\_in\(\), submit\_proof\(work\_hash\), approve\_payout\(worker\), reject\_submission\(worker, reason\), initiate\_dispute\(evidence\_hash\), resolve\_dispute\(in\_favor\_of\) \[arbitrator only ΓÇö NOT creator\], refund\_expired\(\) \[anyone can call\], cancel\_bounty\(\) \[creator only, 0 submissions\]\. Read\-only: get\_bounty\_info\(\), get\_worker\_submission\(worker\), get\_escrow\_balance\(\)\. CRITICAL: resolve\_dispute must check Txn\.sender == App\.globalState\(arbitrator\)\. On approve\_payout, embed txn note 'BountyVault:payout:\{app\_id\}'\. On refund\_expired check either deadline passed OR \(status==DISPUTED AND now > dispute\_deadline\)\.

__2\. BACKEND \(Go/Gin\)__

Structure: cmd/main\.go as entry, internal/\{config,handlers,middleware,models,services,utils\}\. Use environment variable ALGO\_ARBITRATOR\_ADDRESS to set arbitrator at bounty creation\.

Auth handler: register \(email, password, username, role\), login \(JWT \+ refresh\), refresh, me, profile\-update\. Use bcrypt cost 12\. JWT HS256, 24h TTL, 7d refresh\.

Bounty handler endpoints: POST /api/bounties \(create \+ IPFS pin \+ SHA256\), GET /api/bounties \(with query params: tag, status, min\_reward, max\_reward, sort ΓÇö paginated\), GET /api/bounties/:id, POST /api/bounties/:id/lock \(build unsigned txn group\), POST /api/bounties/:id/confirm\-lock, PUT /api/bounties/:id/approve \(approve \+ increment reputation\_score\), PUT /api/bounties/:id/reject, POST /api/bounties/:id/submit \(IPFS \+ on\-chain\), POST /api/bounties/:id/dispute, POST /api/bounties/:id/refund\-expired \(call refund\_expired ABI, no auth required\), GET /api/bounties/:id/submissions \(leaderboard ΓÇö sorted by submission\_time, includes reputation\_score from profiles join\)\.

Algorand service: BuildCreateBountyTxn \(include arbitrator param\), BuildSubmitProofTxn, BuildApprovePayoutTxn \(add txn note 'BountyVault:payout:\{bounty\_uuid\}'\), BuildDisputeTxn, BuildRefundExpiredTxn, BuildCancelTxn\. All txns must include a structured note field: 'BountyVault:\{action\}:\{bounty\_uuid\}'\. WaitForConfirmation after each submission\. Store txn note in transaction\_log\.txn\_note\.

Middleware order: RecoveryMiddleware, RequestIDMiddleware \(UUID\), RequestLoggerMiddleware, SecurityHeadersMiddleware \(X\-Content\-Type\-Options, X\-Frame\-Options, X\-XSS\-Protection, HSTS, CSP\), RateLimitMiddleware \(10rps/20burst general, 5rps/10burst auth, 2rps/4burst upload\), MaxBodySizeMiddleware \(10MB\), CORS, AuthMiddleware\.

__3\. FRONTEND \(Next\.js 15\)__

Home page \(no auth\): Public bounty explorer grid with cards ΓÇö title, reward \(ALGO\), deadline countdown, tag chips, submission count badge, status badge\. Filter bar: tag input, status dropdown, min/max reward, sort selector\. Each card links to bounty detail page\.

Bounty detail page: Full bounty info, terms IPFS link, escrow status, submission leaderboard table \(worker, reputation, submitted, status\)\. Action buttons: Submit Work \(worker, auth required\), Approve/Reject buttons per submission \(creator, auth required\), Trigger Refund button \(visible to all if deadline < now, calls /refund\-expired\), Initiate Dispute button \(creator or worker with submission, auth required\)\.

Wallet integration: Use @perawallet/connect\. Wallet connection triggers opt\_in to the smart contract\. Transaction signing flow: backend returns base64 unsigned txns, frontend decodes, presents to Pera Wallet for signing, sends signed txns back to backend /confirm\-lock endpoint\.

Create bounty form \(creator role, auth required\): title, description, reward \(ALGO, min 1\), deadline picker \(min now\+1hr\), tags \(multi\-input\), max submissions \(1\-50\)\. On submit: calls /api/bounties then redirects to the lock funds step\.

__4\. DATABASE \(Supabase PostgreSQL\)__

Tables: profiles \(id, auth\_user\_id, username, wallet\_address, role, reputation\_score, total\_bounties\_created, total\_bounties\_completed, bio, avatar\_url\), bounties \(id, creator\_id FK, title, description, reward\_algo, terms\_ipfs\_cid, terms\_hash, deadline, status, app\_id, escrow\_txn\_id, arbitrator\_address, max\_submissions, tags\[\]\), submissions \(id, bounty\_id FK, worker\_id FK, work\_ipfs\_cid, work\_hash, description, submission\_txn\_id, status, feedback\), disputes \(id, bounty\_id FK, submission\_id FK, initiated\_by FK, reason, evidence\_ipfs\_cid, status, arbitrator\_address, resolution\_notes, resolved\_at, auto\_refund\_after\), transaction\_log \(id, bounty\_id FK, actor\_id FK, action, txn\_id, txn\_note, amount\_algo, metadata JSONB, created\_at\)\. Enums: user\_role\(creator,worker,admin,arbitrator\), bounty\_status\(open,in\_progress,submitted,completed,disputed,arbitrating,expired,cancelled\), submission\_status\(pending,approved,rejected,disputed\), dispute\_status\(open,resolved\_creator,resolved\_worker,escalated,auto\_refunded\)\.

__CRITICAL CONSTRAINTS__

1\. resolve\_dispute ABI method MUST check sender == arbitrator\. Creator CANNOT call it\. This is the primary trustlessness guarantee\.

2\. refund\_expired MUST also trigger if status == DISPUTED and now > dispute\_deadline \(arbitrator inaction protection\)\.

3\. All Algorand transactions MUST include a note field with format 'BountyVault:\{action\}:\{bounty\_uuid\}'\.

4\. reputation\_score in profiles MUST be incremented when approve\_payout completes successfully\.

5\. contracts/ directory must exist ONLY at root level ΓÇö no duplicate in frontend/\.

6\. Next\.js version is 15\.x ΓÇö do not use 16\.x \(does not exist\)\.

7\. GET /api/bounties must work without authentication and must support tag, status, min\_reward, max\_reward, sort query parameters\.

*BountyVault PRD v2\.0  ΓÇö  Updated April 2026  ΓÇö  AlgoBharat Hackathon*

