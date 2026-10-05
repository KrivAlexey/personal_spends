# 0011 — Transaction identity and deduplication

**Status:** Accepted

## Context

Bank exports overlap: "last month" and "last 90 days" exports share days, and the same file can be uploaded twice by mistake. Each upload must import only transactions not already stored, without losing real ones.

The bank provides no stable transaction ID. A Sparkasse CSV-CAMT row has references that are partly useful: `Sammlerreferenz` carries a card payment's timestamp to the second, but transfers and direct debits have no per-transaction reference, and `Mandatsreferenz` identifies a mandate, not a payment.

Export rows come in two states, given by the `Info` column. **Pending** rows (`Umsatz vorgemerkt`) carry a placeholder merchant code, an internal description and no value date. All of these change when the row is **booked** (`Umsatz gebucht`), sometimes along with the date and amount (card holds, tips).

`Expense.ID` was not generated anywhere yet, so the identity scheme could be chosen without migrating stored data.

## Decision

1. **Booked rows only.** Pending rows are dropped at parse time and counted. Each bank mapping declares its pending marker (`pendingColumn`, `pendingValue`); a bank without one treats every row as booked.
2. **Identity is computed from the bank's raw text.** Each bank mapping lists its `identityColumns`. A row's ID is:
   ```
   sha256( trim(cell₁) ␟ trim(cell₂) ␟ … ␟ trim(cellₙ) )[:16 hex] + "-" + n
   ```
   - The cells are the CSV text after unquoting, before any parsing, joined with the unit separator `\x1f`, which bank text never contains.
   - `n` counts how many identical rows appear earlier **in the same file**, counted over the whole file before any batching. Fully identical rows (fees, same-minute card payments) stay distinct.
   - An export missing any listed identity column is rejected.
3. **Known rows are skipped before categorization.** IDs already stored are looked up first. Only new rows are categorized and written, so re-uploads cost no Claude calls and stored categories don't change.
4. **Expense date is the booking date** (`Buchungstag`) for every row, card payments included, even though card descriptions carry the purchase time.
5. **Storage key:** `SK = <booking date YYYY-MM-DD>#<ID>`. `GSI1SK` uses the same value.

For Sparkasse, `identityColumns` is every column except `Info` (it changes from pending to booked) and `Kategorie` (Sparkasse's own categorization, which the bank can re-run and the owner can edit). `Auftragskonto` is included so identical rows in two accounts' exports stay distinct.

## Alternatives considered

| Option | Rejected because |
|---|---|
| Content hash without a counter | Merges real duplicates: two identical rows on one day become one expense, and money disappears from totals |
| Replace a date range per upload (delete the source's rows in `[min, max]`, then write) | Re-categorizes every row on every upload (Claude cost, categories change between uploads). Delete-then-write isn't atomic, so a crash leaves a gap |
| Hash of the whole file | Catches only byte-identical re-uploads, not overlapping exports |
| Hash of parsed values | Ties identity to our code: a parser fix (amount format, timezone) would change every ID and re-import everything |
| Purchase time as identity or date | Exists only for card payments (inside the description). It would need a second identity path for transfers and debits |
| "All columns except `Info`/`Kategorie`" instead of a list to include | A column Sparkasse adds later would change every hash and re-import all history |

## Consequences

- Re-uploading a file, or uploading an overlapping export, imports only rows not seen before. The upload result reports `imported`, `already_known` and `pending_skipped`; on a re-upload, `imported > 0` means some identity isn't stable.
- Pending transactions appear only after they're booked, in a later export.
- A card payment late on the last day of a month counts toward the month it's booked in.
- A row the bank corrects after booking (changed description, reversal) gets a new ID, so both versions are stored. This is rare and visible, and is fixed by deleting one version by hand.
- Each new bank mapping must choose its identity columns: stable bank-set fields only, nothing the bank or user can edit later.
- Identity is assigned in one pass over the whole file before batching. The worker pool in step 2 ([0003](0003-worker-pool-for-csv.md)) must keep that order.
