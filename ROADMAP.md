# Rodent V — Roadmap

## Goal for first release

- **3200 Elo CCRL** (is 3000)
- Ability to define playing styles ("personalities") in a text file
- Weakening in a sensible range (not necessarily to absolute beginner level)
- HCE eval (for style) supplemented by an auxiliary neural network (for strength boost)

---

## Milestone: Basic Search

- [x] Strict PV node separation in hashing
- [x] Late Move Reduction (one-ply, non-PV nodes)
- [x] Full draw detection (50-move rule, insufficient material)
- [x] Modern PVS (no beta condition)
- [x] History heuristic improvements (like in Chal)

---

## Milestone: Basic Eval

- [x] King safety
- [x] Doubled pawns
- [x] Backward pawns
- [x] King pawn shield
- [-] Rook on 7th rank (fails with current parameters)

---

## Milestone: Tuning round 1

- [x] minimal tuner to assess new eval terms
- [x] Texel tuner with batches
- [x] gradient descent
- [x] tune pst
- [x] tune passers
- [x] tune threats
- [x] tune mobility
- [x] tune material (risky)
- [x] tune whatever remains

---

## Milestone: Easy Search Improvements

- [x] Reverse futility pruning (RFP)
- [x] Late move pruning (LMP)
- [x] Table-driven LMR
- [x] Razoring
- [x] SEE fast-path: `isBadCapture` short-circuits for obviously good captures
      (attacker value ≤ victim value, BxN) before running full SEE

---

## Milestone: Asymmetric Eval + Personalities

- [x] Separate options for own vs. opponent attack and mobility weights
- [x] Define basic option list accessible for personality tuning
- [ ] Load personalities from a text file at startup

---

## Milestone: Regaining eval speed

- [x] Eval hashtable
- [x] Separating eval functions related only to pawns and kings
- [x] Pawn hashtable

---

## Milestone: Small search gains (expect long tuning runs)

- [x] mate distance pruning
- [x] futility pruning

---

## Milestone: advanced search additions

- [x] Singular extensions
- [x] continuation history
- [x] correction history if does not fail
- [x] singular extensions
- [x] multi-threading (lazy SMP)

---

## Milestone: tuning round 2

- [ ] Gradien descent tuner uses batches
- [x] Multi-threaded tuner
- [x] Retune everything with a better set (passed with lichess-quiet, failed with Ethereal)

---

## Milestone: better quiescence search

- [ ] Direct checking moves generator
- [ ] Discovered checks generator
- [ ] (possibly) out of check move generator

## Milestone: advanced eval params

- [ ] Outposts
- [ ] Pawn in front of a minor piece
- [x] Drawish endgames
- [ ] Material imbalances
- [x] Phalanx pawns
- [ ] Defended pawns

## Milestone: user-facing functionalities

- [x] Multi-pv
- [ ] Weakening
- [ ] Personalities presets
- [ ] A tool to tune personalities without too many technical options

## Milestone: beyond standard eval

- [x] Piece/square tables depending on central pawn structure
- [ ] two-loop pawn pairs evaluation

## Longer term

- [x] NNUE auxiliary network for strength boost alongside HCE
- [x] AVX-2 instructions for faster NNUE
- [ ] Online play integration (Go's HTTP support makes this natural)
