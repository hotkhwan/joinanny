# Success Model: ANNY Basic

**Status:** Draft v1.0  
**Target path:** `docs/strategy/success-model-anny-basic.md`  
**Model:** ANNY Basic  
**Initial environment:** Binance Futures Testnet  
**Primary objective:** Prove disciplined, explainable, risk-first trading automation. Not profit marketing.

---

## 1. Purpose

เอกสารนี้นิยามว่า ANNY Basic จะถูกถือว่า “สำเร็จ” เมื่อไร โดยไม่ใช้คำว่า success แบบหลอกตัวเอง

ANNY Basic ไม่ได้สำเร็จเพราะชนะ 1-2 ไม้ ไม่ได้สำเร็จเพราะ win rate สูงใน sample เล็ก และไม่ได้สำเร็จเพราะ screenshot กำไร

ANNY Basic จะถือว่าสำเร็จเมื่อพิสูจน์ได้ว่า:

1. Model ทำงานตาม rule ที่ประกาศไว้
2. Risk control ทำงานก่อน return
3. ทุก mission มี Flight Recorder
4. ทุกผลลัพธ์ตรวจสอบย้อนหลังได้
5. ไม่มีการซ่อน mission ที่แพ้
6. User เป็นคนตัดสินใจและยืนยัน execution
7. ระบบไม่ทำให้ user เข้าใจผิดว่าเป็น financial advice หรือ guaranteed profit

---

## 2. Model Positioning

ANNY Basic คือ baseline model สำหรับ Mission Zero

```text
ANNY Basic is a risk-first AI trading automation model for testing disciplined mission execution, transparent result recording, and user-controlled decision flow.
```

ห้ามใช้ positioning แบบนี้:

```text
profit machine
passive income bot
guaranteed signal
copy trade to win
AI predicts market
safe leverage system
```

---

## 3. Success Definition

ANNY Basic สำเร็จเมื่อผ่าน gate ทั้ง 5 ด้าน

```text
strategyGate
riskGate
executionGate
transparencyGate
legalCommsGate
```

ถ้าด้านใดด้านหนึ่ง fail ห้าม promote เป็น production-ready model

---

## 4. Gate 1: Strategy Gate

Strategy Gate วัดว่า model ทำงานตาม logic ที่นิยามไว้หรือไม่

Minimum criteria:

```text
1. ทุก entry ต้องมี reason ที่ตรวจสอบได้
2. ทุก exit ต้องมี reason ที่ตรวจสอบได้
3. modelVersion ต้องติดกับทุก mission
4. signal evaluation ต้อง reproducible จากข้อมูลที่บันทึกไว้
5. ไม่มี manual override ที่ไม่ถูกบันทึก
6. ไม่มี repaint logic ที่ทำให้ผลย้อนหลังดูดีเกินจริง
```

Required fields:

```json
{
  "modelId": "annyBasic",
  "modelVersion": "1.2.0",
  "signalReason": "cdcQqeAligned",
  "entryDecision": "approvedByUser",
  "exitReason": "takeProfitReached",
  "timeframePrimary": "5m",
  "timeframeConfirm": "15m"
}
```

Fail condition:

```text
ถ้า mission ใดไม่สามารถอธิบายได้ว่าเข้าเพราะอะไร ออกเพราะอะไร ต้องถือว่า mission นั้น invalid สำหรับ model evaluation
```

---

## 5. Gate 2: Risk Gate

Risk Gate สำคัญกว่า profit

Minimum criteria:

```text
1. ทุก mission ต้องมี riskBudgetUsdt ก่อนเปิด position
2. leverage ต้องไม่เกิน maxLeverage ที่ user และ model policy อนุญาต
3. rescue / margin add / averaging rule ต้องถูกจำกัด ไม่ใช่ martingale เปิดไม่จบ
4. max loss ต้องคำนวณได้ก่อนเข้า mission
5. liquidation risk ต้องถูกเตือนก่อน execution
6. user ต้องยืนยัน risk ก่อนเปิด real execution ใน phase ที่อนุญาต
```

Risk object:

```json
{
  "riskMode": "conservative",
  "riskBudgetUsdt": 10,
  "maxLeverage": 20,
  "maxEntryCount": 5,
  "maxRescueCount": 2,
  "maxMarginAddUsdt": 15,
  "requiresUserConfirmation": true
}
```

Hard fail:

```text
- model เพิ่ม position ไม่จำกัด
- model เพิ่ม margin เพื่อถัวไม่จบ
- model เปิด leverage สูงกว่า policy
- model ไม่มี stop condition
- model ใช้คำว่า safe ทั้งที่มี leverage
```

---

## 6. Gate 3: Execution Gate

Execution Gate วัดว่า order flow เสถียรและไม่ทำ mission เพี้ยน

Minimum criteria:

```text
1. order request และ exchange response ถูกบันทึกแบบ sanitize
2. partial fill ต้องถูกจัดการได้
3. failed order ต้องไม่ถูกนับเป็น successful mission
4. SL/TP หรือ exit logic ต้อง sync กับ actual position
5. duplicate order ต้องถูกป้องกันด้วย idempotency key
6. retry ต้องไม่สร้าง order ซ้ำ
7. dryRun/testnet/live ต้องแยกชัดเจน
```

Execution state:

```json
{
  "missionStatus": "closed",
  "executionEnv": "testnet",
  "exchange": "binanceFuturesTestnet",
  "orderState": "filled",
  "positionState": "closed",
  "idempotencyKey": "mis_20260630_000001_entry_001"
}
```

Hard fail:

```text
- mission แสดงกำไรแต่ position ยังไม่ปิดจริง
- order retry แล้วเปิด position ซ้ำ
- failed SL/TP แต่ UI บอกว่ามี protection แล้ว
- env testnet/live ปนกัน
```

---

## 7. Gate 4: Transparency Gate

Transparency Gate คือหัวใจของ ANNY

Minimum criteria:

```text
1. ทุก mission ต้องมี Flight Recorder
2. ทุก closed mission ต้องมี missionHash
3. ทุก result ต้องมี resultHash
4. ทุก recorder ต้องมี recorderHash
5. opBNB testnet anchor ต้องทำงานสำหรับ Mission Zero transparency test
6. proof page ต้องแสดงข้อมูลที่ verify ได้โดยไม่เปิดเผย sensitive data
7. mission แพ้ต้องถูกแสดงใน dataset เท่ากับ mission ชนะ
```

Proof object:

```json
{
  "missionId": "mis_20260630_000001",
  "modelId": "annyBasic",
  "modelVersion": "1.2.0",
  "missionHash": "0x...",
  "modelManifestHash": "0x...",
  "resultHash": "0x...",
  "recorderHash": "0x...",
  "txHash": "0x...",
  "chain": "opbnbTestnet"
}
```

Hard fail:

```text
- ลบ mission แพ้จาก public/internal report
- proof page แสดงเฉพาะ mission ชนะ
- hash ไม่ deterministic
- recorder ถูกแก้หลัง anchor แต่ proof ไม่เปลี่ยนสถานะ
```

---

## 8. Gate 5: Legal and Communication Gate

ANNY Basic ต้องไม่ถูกสื่อสารเป็น investment advice

Required wording:

```text
AI trading companion
risk-first automation
testnet mission
user-controlled execution
not financial advice
no guaranteed profit
```

Forbidden wording:

```text
guaranteed profit
safe income
passive income
copy this model to win
high return with low risk
AI knows the next move
```

Hard fail:

```text
ถ้า landing page, proof page, model card, social post หรือ docs ใช้ profit-first wording ต้อง block release
```

---

## 9. Evaluation Dataset

Mission Zero evaluation ต้องมี sample มากพอ ไม่ใช่เอาแค่เคสสวย ๆ

Minimum target:

```text
completedMissions: 100+
activeBetaUsers: 10+
testDurationDays: 14+
symbols: BTCUSDT and ETHUSDT minimum
marketRegime: trend, range, spike, drawdown
```

Better target:

```text
completedMissions: 300+
activeBetaUsers: 32
testDurationDays: 30+
```

Dataset ต้องรวม:

```text
win missions
loss missions
break-even missions
failed execution missions
skipped missions
manual rejected missions
anchor pending missions
```

---

## 10. Metrics

Metrics แบ่งเป็น 2 กลุ่ม

### 10.1 Public Metrics

แสดงต่อ user ได้

```text
completed missions
win / loss count
average risk per mission
max drawdown per mission
model version
proof coverage percentage
closed mission count
```

### 10.2 Internal Metrics

ใช้สำหรับทีมเท่านั้น ห้ามเอาไป marketing แบบหลอก

```text
winRate
profitFactor
expectancy
averageNetPnl
maxConsecutiveLosses
maxDrawdown
slippage
feeImpact
orderFailureRate
anchorFailureRate
```

Rule:

```text
winRate หรือ profitFactor ห้ามใช้เป็น headline marketing ใน Mission Zero
```

---

## 11. ANNY Basic Score

ใช้ score เพื่อวัดความพร้อม ไม่ใช่เพื่อหลอกขาย

```json
{
  "strategyScore": 0,
  "riskScore": 0,
  "executionScore": 0,
  "transparencyScore": 0,
  "legalCommsScore": 0,
  "overallScore": 0
}
```

Suggested weight:

```json
{
  "strategyScore": 20,
  "riskScore": 30,
  "executionScore": 20,
  "transparencyScore": 20,
  "legalCommsScore": 10
}
```

Promotion rule:

```text
overallScore >= 85
riskScore >= 90
transparencyScore >= 90
legalCommsScore == 100
```

ถ้า legalCommsScore ต่ำกว่า 100 ห้าม promote แม้ผลเทรดดี

---

## 12. Promotion Stages

```text
draft
    ↓
testnetCandidate
    ↓
missionZeroApproved
    ↓
liveCandidate
    ↓
limitedRealTrading
    ↓
deprecated
```

### `draft`

ยังอยู่ใน dev, ไม่เปิดให้ user

### `testnetCandidate`

ใช้กับ Binance testnet ได้ แต่ยังไม่ใช้เป็น public claim

### `missionZeroApproved`

ผ่าน Mission Zero gates และเปิดให้ founder beta ใช้ใน testnet

### `liveCandidate`

ต้องผ่าน security + legal + operational review ก่อนเท่านั้น

### `limitedRealTrading`

ต้องมี user opt-in, real trading gate, risk confirmation และ legal disclosure ครบ

### `deprecated`

หยุดใช้ model version นี้ แต่ยังเก็บ proof/history ไว้

---

## 13. Release Blockers

ห้าม release ถ้ามี blocker ต่อไปนี้

```text
- real trading เปิดได้จาก env เดียว
- user สามารถข้าม risk confirmation ได้
- proof ไม่ถูกสร้างหลัง mission closed
- modelVersion ไม่ถูกบันทึก
- failed order ถูกนับเป็น win
- losing mission ถูก exclude จาก report
- public page มีคำว่า guaranteed profit หรือ passive income
- private strategy logic หลุดใน public proof
- API key หรือ exchange account ถูก log
- opBNB anchor ใช้ wallet ที่ปนกับ treasury หรือ user fund
```

---

## 14. Flight Recorder Required Fields

ทุก mission ต้องมี record ขั้นต่ำ

```json
{
  "missionId": "mis_20260630_000001",
  "modelId": "annyBasic",
  "modelVersion": "1.2.0",
  "executionEnv": "testnet",
  "symbol": "BTCUSDT",
  "side": "long",
  "signalReason": "cdcQqeAligned",
  "riskBudgetUsdt": 10,
  "maxLeverage": 20,
  "userDecision": "approved",
  "entrySummary": {
    "entryCount": 2,
    "averageEntryPrice": 60000
  },
  "exitSummary": {
    "exitReason": "takeProfitReached",
    "averageExitPrice": 60200
  },
  "resultSummary": {
    "grossPnlUsdt": 3.24,
    "feeUsdt": 0.18,
    "netPnlUsdt": 3.06,
    "maxDrawdownUsdt": 1.12
  },
  "proof": {
    "missionHash": "0x...",
    "modelManifestHash": "0x...",
    "resultHash": "0x...",
    "recorderHash": "0x...",
    "txHash": "0x...",
    "chain": "opbnbTestnet"
  }
}
```

---

## 15. Mission Result Classification

อย่านับแค่ win/loss แบบหยาบ ต้องมี classification

```text
win
loss
breakEven
manualRejected
executionFailed
riskRejected
anchorPending
invalidRecord
```

Definition:

```text
win = position closed with positive netPnlUsdt
loss = position closed with negative netPnlUsdt
breakEven = position closed near zero after fees
manualRejected = user rejected suggested mission
executionFailed = order flow failed or position state invalid
riskRejected = model signal appeared but risk policy blocked execution
anchorPending = mission closed but proof anchor not confirmed yet
invalidRecord = recorder incomplete or unreproducible
```

`executionFailed`, `riskRejected`, และ `manualRejected` ต้องไม่ถูกซ่อน เพราะสิ่งเหล่านี้บอกคุณภาพระบบจริง

---

## 16. Mission Zero Success Criteria

Mission Zero ถือว่าสำเร็จเมื่อครบทั้งหมดนี้

```text
1. มี completedMissions อย่างน้อย 100 missions
2. มี activeBetaUsers อย่างน้อย 10 users
3. ทุก mission มี modelVersion
4. ทุก closed mission มี Flight Recorder
5. proofCoverage >= 95%
6. invalidRecordRate <= 2%
7. orderDuplicateIncident = 0
8. envMixIncident = 0
9. secretLeakIncident = 0
10. legalCommsViolation = 0
11. riskRejected missions ถูกบันทึกและแสดงใน internal report
12. losing missions ไม่ถูกลบหรือซ่อน
```

Better target ก่อน Mission One:

```text
completedMissions >= 300
activeBetaUsers >= 32
testDurationDays >= 30
proofCoverage >= 98%
invalidRecordRate <= 1%
```

---

## 17. What Success Is Not

สิ่งต่อไปนี้ไม่ใช่ success ถ้าเกิดเดี่ยว ๆ

```text
- win rate สูงใน sample 20 missions
- กำไรจากช่วงตลาด trend ง่าย ๆ
- social post คนสนใจเยอะ
- founder ชอบ model
- backtest สวยแต่ไม่มี recorder
- testnet กำไรแต่ execution error สูง
- user กดตามโดยไม่เข้าใจ risk
```

---

## 18. Founder Beta Reporting

Report รายสัปดาห์ควรมี

```text
mission count
closed missions
win/loss/breakEven
manualRejected
riskRejected
executionFailed
proofCoverage
anchorFailureRate
averageRiskBudgetUsdt
maxDrawdownUsdt
notable incidents
lessons learned
next model changes
```

ห้าม report เฉพาะกำไร

---

## 19. Go / No-go Checklist

ก่อนประกาศว่า ANNY Basic ผ่าน Mission Zero ต้องตอบได้ว่า yes ทั้งหมด

```text
[ ] Model rules documented
[ ] Risk policy documented
[ ] Flight Recorder complete
[ ] opBNB testnet proof working
[ ] Public proof page sanitized
[ ] Losing missions included
[ ] Execution failures included
[ ] No hidden manual override
[ ] No secret leakage
[ ] No profit-first claim
[ ] No real trading enabled by default
[ ] User confirmation required
[ ] Legal disclaimer visible
[ ] Support and incident path exists
```

---

## 20. Final Rule

ANNY Basic จะไม่ถูกวัดด้วยคำถามว่า “ทำกำไรไหม” เป็นข้อแรก

คำถามแรกต้องเป็น:

```text
Can users trust what ANNY recorded?
```

ถ้าคำตอบคือ no ต่อให้กำไร ก็ยังไม่ผ่าน
