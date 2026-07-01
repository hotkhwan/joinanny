# ANNY Model Template + Auto-Tune Spec (Draft v0.1)

> **Purpose.** วันนี้ ANNY Basic เป็น "โมเดลแข็ง" — ทุก threshold เป็น `const` ใน Go, ปรับที
> ต้อง recompile ทุกครั้ง, และแผนที่ arm ไว้มัก **หมดเวลาโดยไม่ยิง order**. เอกสารนี้นิยาม
> (1) ทำไมไม่ยิง + ปรับตรงไหนให้ยิงจริง, (2) รูปแบบ **Model Template** ที่คน *และ* เครื่องอ่านได้,
> (3) ระบบ **Auto-Tune Recommender** ที่บอกได้ว่า "แผนนี้ไม่ติดเพราะ gate ไหน และควรผ่อนตรงไหน".
>
> Scope = **testnet เท่านั้น** (real trading ปิด). ทุกข้อต้องผ่าน **Legal Gate** ก่อนขึ้น user จริง.
> AI แค่ *เสนอ* — user เป็นคนยืนยันเสมอ (Assessment · Confidence · Suggested Action · user confirms).

---

## 1. ทำไม order ไม่ติด (diagnosis)

Armed watcher เช็คทุก 1 นาที (`armed_mission.go` → `checkArmedMission` → `annyBasicLiveDecision`).
จะได้ side (long/short) ต้อง **ครบพร้อมกัน ณ วินาทีที่เช็ค** (`model.go:88-96` + `noTradeReason`):

| # | เงื่อนไข | ที่มา (const ปัจจุบัน) | ผลต่อความถี่ |
|---|---|---|---|
| 1 | CDC 15m **เพิ่งพลิก**เขียว/แดง ภายใน 1 ชม. | `signalFreshMainBars = 4` (adapter.go:13,32-34) | 🔴 ตัวฆ่าหลัก |
| 2 | QQE **เพิ่ง cross** ทิศเดียวกัน ภายใน 1 ชม. | `recentQQECross(..., 4)` (adapter.go:39) | 🔴 ตัวฆ่าหลัก |
| 3 | QQE value ผ่าน 50 | model.go:90/92 | ปกติ |
| 4 | EMA 1m เรียงตามเทรนด์ | `execFast=5 / execSlow=13` (adapter.go:48) | ปกติ |
| 5 | ไม่ sideways: spread(EMA15m)/price ≥ 0.1% | `< 0.001` (adapter.go:64) | 🟠 ติดบ่อยตอน range |
| 6 | ไม่ extended: |price − fastEMA15m| ≤ 1.5×ATR15m | `1.5` (adapter.go:62) | 🟠 |
| 7 | ไม่ abnormal: TR1m ≤ 3×ATR1m | `3` (adapter.go:63) | นาน ๆ ครั้ง |

**รากปัญหา:** #1 + #2 บังคับให้เหตุการณ์หายาก 2 อย่าง (CDC พลิก + QQE cross) **บรรจบกันในชั่วโมงเดียว**,
และถ้าเทรนด์เขียวมานานเกิน 1 ชม. โค้ด **รีเซ็ต zone เป็น neutral** (adapter.go:32-34) → เทรนด์ที่วิ่งอยู่แล้ว
เข้าไม่ได้ เข้าได้เฉพาะ "ชั่วโมงแรกหลังพลิก". paper จริงเจอ setup ~2 ครั้ง/7 วัน → arm หน้าต่าง 1 ชม.
โอกาสเจอ ~1% ต่อรอบ. ป้าย **"sideways market"** บนจอ = แค่ gate แรกที่บังเอิญติด ณ วินาทีนั้น ไม่ใช่สาเหตุเดียว.

---

## 2. ปรับตรงไหนให้ "ติด" (recommended v1.3 knobs)

เรียงตาม leverage มาก→น้อย. ทุกตัวคือ **การผ่อน gate ที่มีอยู่แล้ว** ไม่ใช่โมเดลใหม่:

| Knob | ตอนนี้ | เสนอ v1.3 | เหตุผล |
|---|---|---|---|
| **CDC-fresh reset** (#1) | รีเซ็ต zone เป็น neutral ถ้าเทรนด์เก่ากว่า 4 บาร์ | **เลิกรีเซ็ต** — ให้เข้าในเทรนด์เขียว/แดงที่ established ได้ ตราบใดที่ QQE เพิ่ง trigger | เข้า "จังหวะ momentum ในเทรนด์" แทน "เฉพาะชั่วโมงแรก" — ถูกต้องกว่าและปลดล็อกความถี่มากสุด |
| **QQE fresh window** (#2) | 4 บาร์ (1 ชม.) | 6–8 บาร์ (1.5–2 ชม.) | ให้ trigger ที่เพิ่งเกิดยังนับได้ |
| **sideways** (#5) | `< 0.001` (0.1%) | `< 0.0005` (0.05%) | บล็อกเฉพาะตอนแบนจริง ๆ |
| #3/#4/#6/#7 | — | **คงไว้** | เป็น safety (กันไล่ราคา/กัน spike) |

> ตัวเดียวที่ควรทำก่อน = **เลิก CDC-fresh reset** (หรือทำ freshness bars ให้เป็น param ค่า default สูงขึ้น).
> คาดว่า fire-rate เพิ่ม 5–10× โดยยังคงคาแรกเตอร์ conservative. ต้องขึ้นเป็น **v1.3** + regen paper results
> + อัปเดต `docs/model/anny-basic.md` + ผ่าน Legal Gate (เพราะเปลี่ยนพฤติกรรมที่ประกาศไว้).

---

## 3. Model Template format (อ่านได้ + เครื่องอ่านได้)

หนึ่งโมเดล = ไฟล์ md เดียว: **การ์ดอธิบาย (มนุษย์)** + **บล็อก params (เครื่อง)**. generalize จาก
`anny-basic.md` ที่มีอยู่. ค่า secret/live ("skill model") **ไม่อยู่ในไฟล์นี้** — อยู่ใน secrets ตามเดิม
(รักษา transparency boundary: เปิดกลไก, ปิดค่าที่จูนแล้วของ live).

```markdown
---
id: anny_basic
version: 1.3
env: testnet          # testnet | live-candidate  (uploaded models = testnet เท่านั้น)
timeframes: { main: 15m, execution: 1m }
rr: { reward: 2, risk: 1 }
params:                # ← ทุกตัวเคยเป็น const ใน Go
  cdc: { fast: 12, slow: 26, freshResetBars: 0 }   # 0 = เลิกรีเซ็ต
  qqe: { rsiPeriod: 14, smooth: 5, factor: 4.236, freshCrossBars: 8, threshold: 50 }
  exec: { emaFast: 5, emaSlow: 13 }
  gates:
    sidewaySpreadPct: 0.0005
    entryExtendedATR: 1.5
    abnormalATR: 3.0
    volumePeriod: 20
  leverage: { fast: 50, momentum: 100, defensive: 50 }
  stops: { profitTargetUsdt: 10, maxTrades: 15, maxConsecutiveLosses: 2 }
signature:            # Phase 4: creator signs → opBNB anchor (ผูกกับ goal-model Step 3)
  creator: null
  txHash: null
---

# <Human-readable model card here — เหมือน anny-basic.md ปัจจุบัน>
```

**Loader:** สร้าง `annybasic.Params` struct ที่ map 1:1 กับ `params:` ข้างบน. โค้ดปัจจุบันอ่าน `const` →
เปลี่ยนเป็นอ่านจาก `Params` (ค่า default = ค่า const เดิม เพื่อ backward-compat + test ไม่พัง).
`Evaluate`/`ObserveAt` รับ `Params` เข้าไป แทน const ฝัง.

---

## 4. Auto-Tune Recommender ("ผ่านแผนแล้วต้องติด")

**Idea:** ทุกครั้งที่ armed check คืน no-trade มันรู้อยู่แล้วว่า gate ไหนบล็อก (`decision.Reason`).
เก็บสถิติ → พอแผนใกล้หมดเวลาแบบไม่ยิง ให้ ANNY *เสนอ* ว่าผ่อนตรงไหนถึงจะติด.

1. **Block histogram** — ระหว่าง watcher loop, นับ reason แต่ละรอบลง mission doc:
   `{ "sideways market": 41, "CDC and QQE not aligned": 12, "entry extended": 7, ... }`.
2. **Recommendation** — ตอน expire (หรือปุ่ม "ทำไมไม่ติด?"): แปลง histogram → คำแนะนำ knob
   จากตาราง §2. ตัวอย่าง: *"71% ของรอบติดที่ sideways gate → ลอง `sidewaySpreadPct` 0.1%→0.05%.
   28% ติดที่ CDC ไม่ fresh → ลอง `freshResetBars` 4→0. ความมั่นใจ: ปานกลาง."*
3. **Fire-rate estimator (paper)** — ก่อน apply, รัน candidate params บน validation window ล่าสุด
   (10080×1m ที่มีอยู่แล้วใน paper engine) → รายงาน *setups/วัน + edge/trade + win-rate ที่คาด*
   เพื่อให้เห็น trade-off (ยิงถี่ขึ้นแลกกับ edge ที่อาจลด) ก่อนตัดสินใจ.
4. **User confirms** — ไม่ auto-apply กับ live model เงียบ ๆ. apply แล้ว = สร้าง **version ใหม่**
   (v1.3.x), log, regen paper. ตรงกับ Legal framing: AI Assessment · Confidence · Suggested · user confirms.

> นี่ทำให้เป้าหมายของคุณเป็นจริง — "เวลาผ่านแผนแล้วมันจะต้องติด order ได้จริง ๆ" — โดยไม่ใช่การ
> hardcode ให้หลวมแบบมั่ว ๆ แต่เป็น **loop: วัด → เสนอ → ประเมินบน paper → user ยืนยัน → ออกเวอร์ชัน**.

---

## 5. Upload / pluggable models

- POST template md → validate schema (§3) → เก็บ Mongo (`models` collection) → โผล่ใน Strategy dropdown.
- Armed watcher/paper engine อ่าน `Params` จาก template ที่เลือก (ไม่ใช่ const).
- **AI ช่วยร่าง:** ให้ LLM ออก template md ตาม format §3 จาก bias/คำอธิบายของ user → user รีวิว → อัปโหลด.
- **opBNB signing (Phase 4):** creator เซ็น model manifest hash → anchor (ผูกกับ [[goal-model-engine]] Step 3).

---

## 6. Guardrails (ห้ามข้าม)

- **Testnet-only** สำหรับ uploaded/tuned models. เข้า live-candidate ต้องผ่าน security + legal + ops review.
- **Hard caps ชนะ template เสมอ:** `missionMaxSizeUSDT=200`, `missionMaxLeverage=100`, `clampLeverage` —
  template ตั้งเกินไม่ได้ (โค้ด clamp อยู่แล้ว, ห้ามถอด).
- **Legal Gate** ทุกข้อความ user-facing: ห้าม guaranteed profit / signal / win-rate เป็น headline.
  Recommender ต้องกรอบว่า "suggested, not advice".
- **Transparency:** ค่า live ที่จูนแล้วไม่หลุดใน public proof; `missionReason` ไม่ควร expose ชื่อโมเดลจริง.
- **Versioning:** ผ่อน gate = bump version + regen paper results + อัปเดต model card. ห้ามแก้เงียบ.

---

## 7. Phased plan + Codex handoff

| Phase | สิ่งที่ทำ | Owned files | Risk |
|---|---|---|---|
| **0 (quick fire fix)** | เลิก CDC-fresh reset + widen QQE fresh 4→8 + sideways→0.0005 เป็น **v1.3** | `internal/strategy/annybasic/{adapter,model,indicators}.go`, `model_test.go`, `docs/model/anny-basic.md` | 🟠 trading behavior + Legal Gate |
| **1 (externalize params)** | สร้าง `annybasic.Params` (default=const เดิม); `Evaluate/ObserveAt` รับ params | annybasic/*.go, mission.go, paper.go | 🟡 refactor, test-covered |
| **2 (recommender)** | block histogram ใน mission doc + endpoint "why no fire" + คำแนะนำ knob | `internal/api/armed_mission.go`, `mission.go`, storage | 🟡 |
| **3 (fire-rate estimator)** | รัน candidate params บน paper window → setups/วัน + edge | `internal/campaign/paper.go`, api | 🟡 |
| **4 (upload + signing)** | template CRUD + dropdown + opBNB manifest sign | api, storage, dashboard, chain | 🔴 legal + chain |

**Handoff เมื่อส่ง Codex:** list changed files, tests run + results, residual risk,
และ flag ทุก path ที่แตะ exchange จริง (ที่นี่ = testnet ผ่าน `orders.ConfirmWithRequiredUserExecutor`).

---

## 8. คำถามที่ต้องตอบก่อนเริ่ม Phase 0

1. เอา Phase 0 (ทำให้ติดเดี๋ยวนี้) แยกทำก่อนเลย หรือรอทำพร้อมระบบ template?
2. ผ่อนแค่ CDC-fresh reset ตัวเดียว (ปลอดภัยสุด) หรือผ่อนทั้ง 3 knob ตาม §2?
3. Phase 0 ให้ผม (Claude) ทำ bounded slice หรือส่ง Codex?
