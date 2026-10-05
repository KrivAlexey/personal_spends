# 0009 — CSV only for v1, no image extraction

**Status:** Accepted

## Context

The original scope included receipt and bill images: upload to S3, extract transactions with Claude Vision, then categorize like CSV rows. `Categorizer` declared an `ExtractFromImage` method for this, but nothing implemented it, so `*Claude` did not satisfy `Categorizer`. Any handler holding a `Categorizer` would not have compiled.

## Decision

v1 accepts bank CSV exports only. `ExtractFromImage` is removed from `Categorizer`, and `*Claude` is checked against the interface at compile time (`var _ Categorizer = (*Claude)(nil)`).

## Rationale

- **Smallest end-to-end path:** CSV upload → parser → categorizer → DynamoDB is enough to exercise every layer. Images add S3, Vision and a second upload flow before the first one works.
- **Interfaces describe what exists:** a method with no implementation breaks every implementation. It comes back when there is code behind it.

## Consequences

- No S3 bucket, `POST /uploads/image` or `S3_BUCKET` in v1.
- Images come back as one backlog item that builds everything together: the S3 bucket in Terraform, the Vision call returning `[]Transaction`, and the method back on `Categorizer`.
