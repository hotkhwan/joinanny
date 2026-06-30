# Mission Zero: opBNB Testnet Transparency Layer

**Status:** Draft v1.0  
**Target path:** `docs/vision/mission-zero-opbnb-testnet.md`  
**Scope:** Mission Zero / ANNY Basic / Flight Recorder / opBNB Testnet  
**Default rule:** Testnet only. No user wallet. No user funds. No token. No staking. No profit claim.

---

## 1. Purpose

Mission Zero ใช้ opBNB testnet เพื่อพิสูจน์ว่า ANNY สามารถสร้าง **public verifiable proof** ให้กับทุก trading mission ได้ โดยไม่เปิดเผยข้อมูลส่วนตัวของผู้ใช้ ไม่เปิดเผย API key ไม่เปิดเผย exchange account และไม่เปิดเผย strategy logic ภายในของ model creator

เป้าหมายของ layer นี้ไม่ใช่การทำเงินบน chain แต่คือการทำให้ผู้ใช้ตรวจสอบได้ว่า:

1. Mission นี้เคยเกิดขึ้นจริง
2. Mission นี้ใช้ model version ไหน
3. Flight Recorder ของ mission นี้ไม่ได้ถูกแก้ย้อนหลังหลังจาก anchor แล้ว
4. ผลลัพธ์ที่แสดงในหน้า proof ตรงกับ hash ที่ถูก anchor
5. ANNY ไม่สามารถลบ mission แพ้ ๆ ออกจากประวัติได้โดยไม่ทิ้งร่องรอย

---

## 2. Non-goals

Mission Zero ห้ามทำสิ่งต่อไปนี้:

- ไม่สร้าง ANNY token
- ไม่ทำ staking
- ไม่ทำ NFT proof
- ไม่รับฝากเงิน user
- ไม่ให้ user เติม BNB
- ไม่ให้ user connect wallet เพื่อเทรด
- ไม่ทำ profit sharing on-chain
- ไม่ขาย copy trade หรือ signal แบบ promise return
- ไม่เอา full order payload ขึ้น chain
- ไม่เอา email, userId จริง, API key, exchange account หรือ personal data ขึ้น chain
- ไม่เปิดเผย private model logic หรือ strategy parameters

---

## 3. Chain Policy

Mission Zero ใช้ `opbnbTestnet` เท่านั้น

```yaml
chainName: opbnbTestnet
chainId: 5611
nativeCurrency: tBNB
walletOwner: annyTransparencyWallet
mainnetAllowed: false
userWalletRequired: false
```

Mainnet จะพิจารณาใน phase หลังจากผ่าน security review, legal review, operational monitoring และ proof verification แล้วเท่านั้น

---

## 4. Wallet Policy

ใช้ wallet กลางของ ANNY สำหรับจ่าย gas เท่านั้น

```text
ANNY Transparency Wallet
```

หน้าที่ของ wallet นี้:

- จ่าย gas สำหรับ anchor proof
- ไม่มีสิทธิ์แตะเงินเทรดของ user
- ไม่มีสิทธิ์ withdraw จาก exchange
- ไม่มี relationship กับ Binance API key ของ user
- เติมเฉพาะ testnet token ใน Mission Zero

Private key policy:

```text
localDev: ใช้ test wallet ได้
flyDev: เก็บผ่าน Fly secrets เท่านั้น
production: ห้ามใช้ private key ตรง ๆ ใน .env แบบ unmanaged
mainnet: ต้องแยก signer หรือใช้ KMS/Vault/hardware-backed signer ก่อนเปิดใช้
```

---

## 5. Architecture

```text
Mission Closed
    ↓
Create Canonical Flight Recorder Snapshot
    ↓
Hash mission / model / result / recorder
    ↓
Submit anchor transaction to opBNB testnet
    ↓
Receive txHash
    ↓
Store proof metadata in MongoDB
    ↓
Expose public proof page
```

Core separation:

```text
MongoDB = private source of truth
opBNB = public timestamp and immutability anchor
Fly.io = application runtime
Cloudflare = edge, DNS, SSL/TLS, caching, security rules
Resend = email verification and beta communication
```

---

## 6. Proof Object

Public proof object สำหรับ Mission Zero ควรมี field เท่านี้ก่อน

```json
{
  "schemaVersion": "1.0",
  "proofType": "missionResult",
  "chain": "opbnbTestnet",
  "missionHash": "0x...",
  "modelManifestHash": "0x...",
  "resultHash": "0x...",
  "recorderHash": "0x...",
  "timestamp": 1780000000
}
```

Field ที่ยังไม่ต้องใส่ใน Mission Zero แต่เตรียมไว้สำหรับ Mission One:

```json
{
  "modelArtifactHash": "0x...",
  "creatorHash": "0x...",
  "userHash": "0x..."
}
```

---

## 7. Hash Rules

Hash ต้อง deterministic และ replay ได้

Minimum rule:

1. ใช้ canonical JSON
2. ใช้ key order แบบ stable
3. timestamp เป็น UTC หรือ Unix timestamp เท่านั้น
4. decimal ต้อง normalize ก่อน hash
5. ห้าม hash object ที่มี field เปลี่ยนเอง เช่น `updatedAt`, random order, transient error text
6. ห้ามรวม secret ลงใน snapshot

Recommended hash function:

```text
keccak256(canonicalJsonBytes)
```

เหตุผล: ใช้กับ EVM / Solidity / event `bytes32` ได้ตรงกว่า

---

## 8. Mission Snapshot

ตัวอย่าง mission snapshot ที่นำไป hash ภายใน backend

```json
{
  "schemaVersion": "1.0",
  "missionId": "mis_20260630_000001",
  "modelId": "annyBasic",
  "modelVersion": "1.2.0",
  "exchange": "binanceFuturesTestnet",
  "symbol": "BTCUSDT",
  "side": "long",
  "riskProfile": {
    "riskMode": "conservative",
    "maxLeverage": 20,
    "riskBudgetUsdt": 10,
    "requiresUserConfirmation": true
  },
  "executionSummary": {
    "entryCount": 2,
    "exitReason": "takeProfitReached",
    "status": "closed"
  },
  "resultSummary": {
    "grossPnlUsdt": 3.24,
    "feeUsdt": 0.18,
    "netPnlUsdt": 3.06,
    "maxDrawdownUsdt": 1.12
  },
  "closedAt": "2026-06-30T00:00:00Z"
}
```

Public proof page แสดงได้เฉพาะ summary ที่ sanitize แล้ว ห้ามแสดง raw order payload หรือ private model parameters

---

## 9. Smart Contract MVP

Mission Zero ใช้ contract ง่ายที่สุดพอ

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract AnnyTransparencyAnchor {
    event MissionAnchored(
        bytes32 indexed missionHash,
        bytes32 indexed modelManifestHash,
        bytes32 resultHash,
        bytes32 recorderHash,
        uint256 anchoredAt
    );

    function anchorMission(
        bytes32 missionHash,
        bytes32 modelManifestHash,
        bytes32 resultHash,
        bytes32 recorderHash
    ) external {
        emit MissionAnchored(
            missionHash,
            modelManifestHash,
            resultHash,
            recorderHash,
            block.timestamp
        );
    }
}
```

Mission Zero ยังไม่ต้องมี access control ซับซ้อนถ้า contract ใช้ testnet เท่านั้น แต่ backend ต้องจำกัดว่าใครสามารถเรียก anchor ได้

สำหรับ mainnet ต้องเพิ่ม owner/role, pause, event versioning และ deployment registry

---

## 10. MongoDB Collections

### `missionProofs`

```json
{
  "missionId": "mis_20260630_000001",
  "modelId": "annyBasic",
  "modelVersion": "1.2.0",
  "proofType": "missionResult",
  "chain": "opbnbTestnet",
  "chainId": 5611,
  "contractAddress": "0x...",
  "txHash": "0x...",
  "missionHash": "0x...",
  "modelManifestHash": "0x...",
  "resultHash": "0x...",
  "recorderHash": "0x...",
  "anchorStatus": "confirmed",
  "anchorAttempts": 1,
  "anchoredAt": "2026-06-30T00:00:00Z",
  "createdAt": "2026-06-30T00:00:00Z"
}
```

### `proofAnchorJobs`

```json
{
  "jobId": "job_20260630_000001",
  "missionId": "mis_20260630_000001",
  "status": "pending",
  "attemptCount": 0,
  "nextRunAt": "2026-06-30T00:00:00Z",
  "lastError": null,
  "createdAt": "2026-06-30T00:00:00Z"
}
```

---

## 11. Backend Service

Target package:

```text
internal/services/transparencysvc
```

Required functions:

```text
createCanonicalMissionSnapshot
createMissionHash
createResultHash
createRecorderHash
createModelManifestHash
anchorMissionProof
verifyMissionProof
getPublicMissionProof
```

Internal endpoints:

```text
POST /internal/transparency/missions/:missionId/anchor
GET  /internal/transparency/missions/:missionId/verify
```

Public endpoints:

```text
GET /proof/missions/:missionId
```

Public endpoint ห้าม return private payload

---

## 12. Environment Variables

```env
TRANSPARENCY_ENABLED=true
TRANSPARENCY_CHAIN=opbnbTestnet
TRANSPARENCY_DRY_RUN=false
OPBNB_CHAIN_ID=5611
OPBNB_RPC_URL=https://...
OPBNB_CONTRACT_ADDRESS=0x...
OPBNB_RELAYER_PRIVATE_KEY=...
OPBNB_TX_CONFIRMATIONS=1
OPBNB_ANCHOR_TIMEOUT_SECONDS=60
```

Rules:

```text
TRANSPARENCY_ENABLED=true ใช้ได้ใน dev/testnet
TRANSPARENCY_DRY_RUN=true ใช้สำหรับ local test
OPBNB_RELAYER_PRIVATE_KEY ห้าม commit
OPBNB_RPC_URL ห้าม hardcode ถ้าเป็น provider key ส่วนตัว
```

---

## 13. Public Proof Page

Route:

```text
/proof/missions/:missionId
```

แสดง:

```text
Mission ID
Model name
Model version
Market
Symbol
Risk mode
Mission status
Result summary
Mission hash
Model manifest hash
Result hash
Flight recorder hash
opBNB txHash
Explorer link
```

ต้องมี disclaimer:

```text
This proof verifies that the mission record existed at the anchored time and that the displayed summary matches the recorded hashes.
It does not guarantee future profit.
It is not financial advice.
```

---

## 14. Retry and Failure Policy

Anchor failure ห้ามทำให้ trading mission หาย

```text
missionClosed = true
proofAnchorStatus = pending | confirmed | failed | skipped
```

Retry rules:

```text
maxAttempts: 5
backoff: exponential
retryableErrors: rpcTimeout, nonceTooLow, replacementUnderpriced, temporaryRpcError
nonRetryableErrors: invalidContractAddress, invalidPrivateKey, invalidHashFormat
```

ถ้า anchor ไม่สำเร็จ ต้องแสดงบน internal dashboard และ proof page ว่า `proof pending` หรือ `proof failed` ไม่ใช่ซ่อนเงียบ

---

## 15. Security Rules

ห้ามทำ:

- ห้าม commit private key
- ห้าม log private key
- ห้าม log raw exchange payload ที่มี API key หรือ signature
- ห้าม expose raw recorder snapshot ถ้ามี sensitive field
- ห้ามให้ public user เรียก anchor endpoint ตรง
- ห้ามให้ user เลือก arbitrary hash เพื่อ anchor เองใน Mission Zero

ต้องทำ:

- sanitize proof response
- add audit log for anchor attempts
- validate mission ownership internally
- validate model version exists
- validate mission is closed before anchor
- rate limit public proof endpoint
- cache public proof endpoint ได้ แต่ต้องไม่ cache private API

---

## 16. Mission Zero Acceptance Criteria

Mission Zero จะถือว่าผ่าน opBNB transparency gate เมื่อครบเงื่อนไขนี้:

```text
1. ANNY Basic mission ปิดแล้วสามารถสร้าง canonical snapshot ได้
2. missionHash, modelManifestHash, resultHash, recorderHash deterministic และ verify ซ้ำได้
3. backend สามารถ anchor proof ไป opBNB testnet ได้
4. txHash ถูกบันทึกใน MongoDB
5. public proof page แสดง hash + txHash ได้โดยไม่เปิดเผย sensitive data
6. proof verification endpoint สามารถยืนยันว่า displayed proof ตรงกับ on-chain event
7. anchor failure มี retry และไม่ทำให้ mission record หาย
8. ไม่มี user wallet requirement
9. ไม่มี user fund custody
10. ไม่มี profit guarantee wording ในหน้า proof
```

---

## 17. Rollout Plan

### Step 1: Dry Run

```text
- สร้าง canonical snapshot
- สร้าง hashes
- เก็บใน MongoDB
- ยังไม่ส่ง tx
```

### Step 2: Testnet Anchor

```text
- deploy contract บน opBNB testnet
- เติม tBNB ให้ ANNY Transparency Wallet
- ส่ง anchor transaction หลัง mission closed
- เก็บ txHash
```

### Step 3: Public Proof

```text
- เปิด /proof/missions/:missionId
- แสดง proof summary
- link explorer
- verify hash locally
```

### Step 4: Beta Review

```text
- review 100+ missions
- review proof failure rate
- review sensitive data leakage
- review user understanding
```

---

## 18. Mission One Preparation

Mission One Model Market ต้องเพิ่ม proof type ใหม่:

```json
{
  "schemaVersion": "1.0",
  "proofType": "modelRegistration",
  "chain": "opbnbTestnet",
  "modelManifestHash": "0x...",
  "modelArtifactHash": "0x...",
  "creatorHash": "0x...",
  "timestamp": 1780000000
}
```

แต่ Mission Zero ยังไม่ต้องเปิด creator marketplace

---

## 19. Final Rule

opBNB ใน Mission Zero คือ transparency tool เท่านั้น

```text
No token.
No staking.
No custody.
No guaranteed return.
No user wallet requirement.
Proof only.
```
