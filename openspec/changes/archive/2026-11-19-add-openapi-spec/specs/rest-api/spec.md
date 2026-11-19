## ADDED Requirements

### Requirement: Serve the OpenAPI document
The system SHALL respond to `GET /openapi.yaml` with `200`, `Content-Type: application/yaml` and an OpenAPI 3.1 document that describes every route the server serves, including `GET /openapi.yaml` itself, and no route it does not serve.

#### Scenario: Fetch the document
- **WHEN** a client sends `GET /openapi.yaml`
- **THEN** the response is `200` with `Content-Type: application/yaml` and an OpenAPI 3.1 document listing `POST /uploads/csv` and `GET /openapi.yaml`

#### Scenario: Wrong method
- **WHEN** a client sends `POST /openapi.yaml`
- **THEN** the response is `405`
