# appconfig-cache

> Read this in: [Português](README.pt-br.md) | [English](README.md)

A Go-based AWS Lambda for fetching configurations from AWS AppConfig using a cache-aside strategy.

## Project Documentation

For detailed business and technical architecture information, see:
- [Business Documentation (Cost Impact and Savings)](docs/business.md)
- [Architecture Documentation (3-Tier Cache and Technical Flow)](docs/architecture.md)

## Code Structure

- `cmd/lambda`: AWS Lambda entrypoint (API Gateway contract)
- `cmd/local`: Local runner for manual testing
- `cmd/server`: Local HTTP server for testing/integration
- `internal/domain`: Domain objects
- `internal/application`: Use cases and ports
- `internal/infrastructure`: Adapters for AWS/Valkey
- `internal/bootstrap`: Dependency injection/composition

## Environment Variables

Use the `.env` file as a template:

- `AWS_REGION`
- `VALKEY_HOST` (optional; takes precedence if set)
- `VALKEY_PORT` (optional; defaults to `6379` when missing/empty)
- `CACHE_SECRET_NAME`
- `L1_TTL_SECONDS`
- `L2_TTL_SECONDS`
- `X_API_KEY` (protects the `/v1/config` endpoint in `cmd/server`)
- `CIRCUIT_BREAKER_TABLE_NAME` (optional; enables shared circuit breaker in DynamoDB)

> If `VALKEY_HOST` is defined, the service uses environment variables for Valkey (with a default `VALKEY_PORT` of `6379`) and will not query the Secrets Manager.

When `X_API_KEY` is defined, authentication accepts:

- `x-api-token` header
- `x-api-token` query string parameter

### Expected Secret Format in Secrets Manager

```json
{
  "host": "my-valkey.xxxxxx.use1.cache.amazonaws.com",
  "port": 6379
}
```

## Local Usage

Prerequisite: Valid AWS credentials configured in your environment.

If running on a machine or instance with an IAM Role (e.g., EC2), run with an SSO profile to avoid falling back to the instance role:

```bash
aws sso login --profile <your_sso_profile>
AWS_PROFILE=<your_sso_profile> make run-local APP=<app_id> ENV=<env_id> PROFILE=<profile_id>
```

```bash
make fmt
make test
make run-local APP=<app_id> ENV=<env_id> PROFILE=<profile_id>
make run-server ADDR=:8080
```

## Running as a Server

Start the server:

```bash
make run-server ADDR=:8080
```

Healthcheck:

```bash
curl http://localhost:8080/healthz
```

Get configuration (GET):

```bash
curl "http://localhost:8080/v1/config?application=<app_id>&environment=<env_id>&profile=<profile_id>"

# alternative via query string API key:
curl "http://localhost:8080/v1/config?application=<app_id>&environment=<env_id>&profile=<profile_id>&x-api-token=<x_api_key>"
```

Get configuration (POST):

```bash
curl -X POST http://localhost:8080/v1/config \
  -H "Content-Type: application/json" \
  -H "x-api-token: <x_api_key>" \
  -d '{"application":"<app_id>","environment":"<env_id>","profile":"<profile_id>"}'
```

## Lambda Contract (API Gateway)

The handler accepts `application`, `environment`, and `profile` in:

1. Query string parameters (`?application=...&environment=...&profile=...`)
2. Path parameters (`{application}`, `{environment}`, and `{profile}`)
3. JSON Body (`{"application":"...","environment":"...","profile":"..."}`)

If `X_API_KEY` is configured on the Lambda, you must also send:

- `x-api-token` header **or**
- `x-api-token` query string parameter

The response is always JSON:

- **200 OK**

```json
{
  "example_feature_flag": {
    "enabled": true
  }
}
```

> On success, the service returns the AppConfig document directly (without a wrapping `configuration` object).

- **400/500 Error**

```json
{
  "message": "..."
}
```

## Shared Circuit Breaker (Lambda)

To enable the shared circuit breaker based on DynamoDB, add the following to your `.env` file:

```bash
CIRCUIT_BREAKER_TABLE_NAME=appconfig-circuit-breaker
```

For operation details, a guide on creating the DynamoDB table, and CLI monitoring commands, see the [Circuit Breaker Section in the Architecture Documentation](docs/architecture.md#4-shared-circuit-breaker-dynamodb).

## Optimized Docker (Resource-Constrained Environments)

The project includes a multi-target `Dockerfile`:

- `server-runtime`: A lightweight image to run `cmd/server` as a non-root user.
- `k6-runner`: An image containing load testing scripts under `scripts/k6`.

### Building the Server Image

```bash
docker build --target server-runtime -t appconfig-cache:server .
```

### Running the Server with Resource Limits

```bash
docker run --rm \
  --name appconfig-cache \
  --cpus="0.50" \
  --memory="256m" \
  -p 8080:8080 \
  --env-file .env \
  appconfig-cache:server
```

### Building the k6 Image

```bash
docker build --target k6-runner -t appconfig-cache:k6 .
```

### CPU-bound k6 Test

`scripts/k6/cpu_bound_test.js` forces CPU load on the load generator (repeated hashing), which is useful to validate behavior when k6 itself becomes the bottleneck.

```bash
docker run --rm \
  --cpus="0.50" \
  --memory="256m" \
  appconfig-cache:k6 run /k6/scripts/cpu_bound_test.js \
  -e VUS=2 \
  -e DURATION=45s \
  -e ROUNDS_PER_ITERATION=15000
```

### IO-bound k6 Test

`scripts/k6/io_bound_test.js` prioritizes network/IO wait, making HTTP requests to `/v1/config`.

```bash
docker run --rm \
  --cpus="0.50" \
  --memory="256m" \
  --add-host=host.docker.internal:host-gateway \
  appconfig-cache:k6 run /k6/scripts/io_bound_test.js \
  -e TARGET_URL="http://host.docker.internal:8080/v1/config?application=<app_id>&environment=<env_id>&profile=<profile_id>" \
  -e X_API_KEY="<optional_x_api_key>" \
  -e RATE=20 \
  -e PARALLEL_REQUESTS=4
```

> Tip: If your goal is to stress the application (not k6), keep the `cpu_bound_test`'s `ROUNDS_PER_ITERATION` low and prioritize `io_bound_test`.

## Building for Lambda

```bash
make build-lambda
```

Generated artifact:

- `dist/lambda.zip`

## Deployment Notes

- Recommended runtime: `provided.al2023`
- Handler: `bootstrap` (the binary inside the zip)
- Current build architecture: `amd64`
