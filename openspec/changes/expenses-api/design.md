## Decisions

- **Query keys.** No category: Query `PK = EXPENSES` and `SK BETWEEN <from> AND <to>#~` on the base table. With a category: Query GSI1 with `GSI1PK = CAT#<category>` and the same range on `GSI1SK`. `~` sorts after every character an ID holds. A missing `from` or `to` leaves that end open.
- **Amount bounds** are a filter expression on `amount`; DynamoDB applies it after reading, so `limit` counts matches, and QueryExpenses follows `LastEvaluatedKey` until it has enough.
- **Order** is newest first (`ScanIndexForward=false`).
- **`limit`** defaults to 100 and is capped at 500. The summary reads every expense in the range (`Limit` 0 in the query), since a truncated total would be wrong. The table holds one person's spending, so the range stays small.
- **Summary amounts** are rounded to cents after summing, so float noise doesn't show.
- **POST /expenses** takes `date`, `amount`, `merchant`, `category` and optional `raw_description`. The category must be a configured one. The server sets `currency=EUR` (0010), `confidence=1`, `source=manual` and a random ID of the form `<16 hex>-0`, so an identical second POST adds a second expense (two identical purchases are legitimate).
- **Errors** follow the upload endpoints: `400` with fixed text for bad input, `500` with `internal error` and the chain logged once.
