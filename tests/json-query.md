# PRD: JSON Query API (`mem_json_query`)

## Summary

Add a `mem_json_query` MCP tool to **mem** that lets AI agents run SQL-filtered queries
against memory entities (events, decisions, lessons, sessions) using a JSON criteria
payload. Supports logical combinators (OR, NOT), common SQL operators, ordering, and
pagination — all safe from injection.

## Motivation

Agents currently retrieve data via `mem_get_details` (single entity) and
`mem_search_lessons` (FTS5 on lessons only). There is no general-purpose query tool for
events, decisions, or sessions. `mem_json_query` fills this gap.

## Requirements

- Accept `entity` (events, decisions, lessons, sessions), `filters`, `order_by`, `limit`, `offset`
- Whitelist queryable columns per entity — no raw SQL injection
- Whitelist allowed operators: `eq`, `neq`, `contains`, `gt`, `gte`, `lt`, `lte`, `between`, `in`, `is_null`, `not_null`
- Support `or` filter array (any match) and `not` filter (negation)
- Clamp `limit` to 50–200, default 50
- Validate `order_by` against safe regex
- Return `entity`, `results`, `count`, `limit`, `offset`

## Deliverables

### `tests/json-query/query_examples.json`
Collection of valid and invalid JSON query payloads for documentation and testing.

### `tests/json-query/e2e_test.go`
End-to-end Go test that starts an in-memory mem store, creates sample data across all
entity types, and exercises every operator and combinator on every entity.

## Implementation files
- [x] `store/json_query.go` — core query builder
- [x] `store/json_query_test.go` — unit tests
- [ ] `tests/json-query/query_examples.json` — example payloads
- [ ] `tests/json-query/e2e_test.go` — end-to-end integration tests
