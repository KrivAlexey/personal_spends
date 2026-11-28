## ADDED Requirements

### Requirement: Expense fields
Every expense in a response SHALL have the fields `id`, `date` (`YYYY-MM-DD`), `amount` (number), `currency`, `merchant`, `raw_description`, `category`, `confidence`, `source` and `created_at` (RFC 3339), all always present.

#### Scenario: Stored expense
- **WHEN** an expense with every field set is stored and listed
- **THEN** the listed element has all ten fields

### Requirement: Query expenses
The system SHALL answer `GET /expenses` with `200` and a JSON body `{"expenses": [...]}`, newest booking date first, each element in the shape of "Expense fields". The optional parameters narrow the result: `from` and `to` (`YYYY-MM-DD`, both inclusive) bound the booking date, `category` keeps one category, `min_amount` and `max_amount` (inclusive, signed decimals) bound the amount, and `limit` caps the number of expenses (default 100, at most 500). With no match the list SHALL be empty.

#### Scenario: Date range
- **WHEN** expenses dated 2026-03-31, 2026-04-01, 2026-04-30 and 2026-05-01 are stored and `GET /expenses?from=2026-04-01&to=2026-04-30` is requested
- **THEN** the response lists the 2026-04-30 expense, then the 2026-04-01 one

#### Scenario: Category and amount bounds
- **WHEN** `GET /expenses?category=groceries&min_amount=-50&max_amount=-10` is requested and groceries expenses of -5, -20 and -60 are stored
- **THEN** only the -20 expense is listed

#### Scenario: Limit
- **WHEN** 3 expenses match and `limit=2` is requested
- **THEN** the 2 newest are listed

#### Scenario: No match
- **WHEN** nothing is stored
- **THEN** the response is `200` with `{"expenses": []}`

### Requirement: Invalid expense queries are rejected
The system SHALL respond `400` when `from` or `to` is not a date, `from` is after `to`, `min_amount` or `max_amount` is not a number, `min_amount` is above `max_amount`, or `limit` is not an integer from 1 to 500. Nothing SHALL be read from storage.

#### Scenario: Bad date
- **WHEN** `GET /expenses?from=yesterday` is requested
- **THEN** the response is `400`

#### Scenario: Inverted range
- **WHEN** `GET /expenses?from=2026-05-01&to=2026-04-01` is requested
- **THEN** the response is `400`

#### Scenario: Limit out of range
- **WHEN** `GET /expenses?limit=501` is requested
- **THEN** the response is `400`

### Requirement: Add an expense
The system SHALL accept `POST /expenses` with a JSON body holding `date` (`YYYY-MM-DD`), `amount` (number, not 0), `merchant` (non-empty), `category` and an optional `raw_description`, store it, and respond `201` with the stored expense in the shape of "Expense fields". The category MUST be a configured one. The expense SHALL then appear in `GET /expenses`.

#### Scenario: Valid expense
- **WHEN** `{"date":"2026-04-02","amount":-12.5,"merchant":"Bakery","category":"groceries"}` is posted
- **THEN** the response is `201` with `source` `manual`, `currency` `EUR`, `confidence` `1` and a non-empty `id`, and the expense is stored

#### Scenario: Same body twice
- **WHEN** the same body is posted twice
- **THEN** both responses are `201` with different ids

### Requirement: Server-set fields of an added expense
The system SHALL set `currency` to `EUR`, `confidence` to `1`, `source` to `manual`, `created_at` to the current time and `id` to a new unique value on an expense added with `POST /expenses`.

#### Scenario: Fields set
- **WHEN** a valid expense is posted
- **THEN** the stored expense has `currency` `EUR`, `confidence` `1`, `source` `manual`, a `created_at` within a minute of now and a non-empty `id`

### Requirement: Invalid expenses are rejected
The system SHALL respond `400` and store nothing when the body is not a JSON object, has an unknown field, lacks `date`, `amount`, `merchant` or `category`, has an unparseable date, a zero amount or an empty merchant, or names a category that is not configured. A body over 64 KiB SHALL be answered `413`.

#### Scenario: Unknown category
- **WHEN** a body with `"category":"nonsense"` is posted
- **THEN** the response is `400` and nothing is stored

#### Scenario: Missing merchant
- **WHEN** a body without `merchant` is posted
- **THEN** the response is `400` and nothing is stored

#### Scenario: Not JSON
- **WHEN** `hello` is posted
- **THEN** the response is `400` and nothing is stored

### Requirement: Spending summary
The system SHALL answer `GET /expenses/summary` with `200` and a JSON body `{"categories": [...], "total": <number>, "count": <integer>}` where each element has `category`, `total` (sum of amounts, rounded to cents) and `count`, sorted by category name. The top-level `total` and `count` cover all expenses in the range. The optional `from` and `to` bound the booking date as in "Query expenses" and are rejected with `400` under the same rules.

#### Scenario: Two categories
- **WHEN** expenses of -10.10 and -20.20 in `groceries` and -5 in `fuel` are stored and `GET /expenses/summary` is requested
- **THEN** the response lists `fuel` with total -5 and count 1, then `groceries` with total -30.3 and count 2, and `total` is -35.3 and `count` 3

#### Scenario: Empty range
- **WHEN** nothing is stored in the range
- **THEN** the response is `200` with `{"categories": [], "total": 0, "count": 0}`

### Requirement: List categories
The system SHALL answer `GET /categories` with `200` and a JSON body `{"categories": [{"name": ..., "description": ...}]}` listing the configured categories.

#### Scenario: Configured categories
- **WHEN** the categories `groceries` and `fuel` are configured and `GET /categories` is requested
- **THEN** the response lists both with their descriptions

### Requirement: Expense endpoint failures do not leak internals
The system SHALL respond `500` when reading or writing expenses, or listing categories, fails, and the response body SHALL NOT contain the internal error message.

#### Scenario: Storage failure
- **WHEN** reading expenses fails with an error naming the DynamoDB table
- **THEN** the response is `500` and its body does not contain the table name
