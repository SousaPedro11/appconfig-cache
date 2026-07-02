# appconfig-cache

> Read this in: [English](README.md) | [Português](README.pt-br.md)

Lambda em Go para buscar configuração do AWS AppConfig com estratégia cache-aside.

## Documentação do Projeto

Para informações detalhadas de negócio e arquitetura técnica, consulte:
- [Documentação de Negócio (Impacto de Custo e Redução de Gastos)](docs/pt-br/business.md)
- [Documentação de Arquitetura (Cache de 3 Níveis e Fluxo Técnico)](docs/pt-br/architecture.md)

## Estrutura do Código

- `cmd/lambda`: entrypoint AWS Lambda (contrato API Gateway)
- `cmd/local`: runner local para teste/manual
- `cmd/server`: servidor HTTP local para integração/teste
- `internal/domain`: objetos de domínio
- `internal/application`: caso de uso + portas
- `internal/infrastructure`: adapters AWS/Valkey
- `internal/bootstrap`: composição das dependências

## Variáveis de ambiente

Use o arquivo `.env` como base:

- `AWS_REGION`
- `VALKEY_HOST` (opcional; se definido, tem prioridade)
- `VALKEY_PORT` (opcional; default `6379` quando ausente/vazio)
- `CACHE_SECRET_NAME`
- `L1_TTL_SECONDS`
- `L2_TTL_SECONDS`
- `X_API_KEY` (protege o endpoint `/v1/config` do `cmd/server`)
- `CIRCUIT_BREAKER_TABLE_NAME` (opcional, ativa circuit breaker compartilhado em DynamoDB)

> Se `VALKEY_HOST` estiver definida, o serviço usa variáveis de ambiente para o Valkey (com `VALKEY_PORT` default `6379`) e não consulta o Secrets Manager.

Quando `X_API_KEY` estiver definida, a autenticação aceita:

- header `x-api-token`
- querystring `x-api-token`

### Formato esperado do secret no Secrets Manager

```json
{
  "host": "meu-valkey.xxxxxx.use1.cache.amazonaws.com",
  "port": 6379
}
```

## Uso local

Pré-requisito: credenciais AWS válidas no ambiente.

Se estiver em máquina/instância com IAM Role (ex.: EC2), rode com perfil SSO para evitar fallback na role da instância:

```bash
aws sso login --profile <seu_profile_sso>
AWS_PROFILE=<seu_profile_sso> make run-local APP=<app_id> ENV=<env_id> PROFILE=<profile_id>
```

```bash
make fmt
make test
make run-local APP=<app_id> ENV=<env_id> PROFILE=<profile_id>
make run-server ADDR=:8080
```

## Rodando como server

Subir servidor:

```bash
make run-server ADDR=:8080
```

Healthcheck:

```bash
curl http://localhost:8080/healthz
```

Buscar configuração (GET):

```bash
curl "http://localhost:8080/v1/config?application=<app_id>&environment=<env_id>&profile=<profile_id>"

# alternativa via querystring:
curl "http://localhost:8080/v1/config?application=<app_id>&environment=<env_id>&profile=<profile_id>&x-api-token=<x_api_key>"
```

Buscar configuração (POST):

```bash
curl -X POST http://localhost:8080/v1/config \
  -H "Content-Type: application/json" \
  -H "x-api-token: <x_api_key>" \
  -d '{"application":"<app_id>","environment":"<env_id>","profile":"<profile_id>"}'
```

## Contrato da Lambda (API Gateway)

O handler aceita `application`, `environment` e `profile` em:

1. query string (`?application=...&environment=...&profile=...`)
2. path parameters (`{application}`, `{environment}` e `{profile}`)
3. body JSON (`{"application":"...","environment":"...","profile":"..."}`)

Se `X_API_KEY` estiver configurada na Lambda, enviar também:

- header `x-api-token` **ou**
- querystring `x-api-token`

A resposta é sempre JSON:

- **200**

```json
{
  "feature_flag_exemplo": {
    "enabled": true
  }
}
```

> Em sucesso, o serviço retorna o documento do AppConfig diretamente (sem envelope `configuration`).

- **400/500**

```json
{
  "message": "..."
}
```

## Circuit Breaker Compartilhado (Lambda)

Para ativar o circuit breaker compartilhado baseado em DynamoDB, adicione ao seu arquivo `.env`:

```bash
CIRCUIT_BREAKER_TABLE_NAME=appconfig-circuit-breaker
```

Para detalhes sobre funcionamento, guia de criação da tabela no DynamoDB e comandos de monitoramento via CLI, consulte a [Seção de Circuit Breaker na Documentação de Arquitetura](docs/architecture.md#4-circuit-breaker-compartilhado-dynamodb).

## Docker otimizado (ambiente com recurso limitado)

O projeto agora possui um `Dockerfile` multi-target com:

- `server-runtime`: imagem leve para rodar `cmd/server` como usuário não-root
- `k6-runner`: imagem com scripts de carga em `scripts/k6`

### Build da imagem do servidor

```bash
docker build --target server-runtime -t appconfig-cache:server .
```

### Subir servidor com limite de recurso

```bash
docker run --rm \
  --name appconfig-cache \
  --cpus="0.50" \
  --memory="256m" \
  -p 8080:8080 \
  --env-file .env \
  appconfig-cache:server
```

### Build da imagem k6

```bash
docker build --target k6-runner -t appconfig-cache:k6 .
```

### Teste k6 CPU-bound

`scripts/k6/cpu_bound_test.js` força trabalho de CPU no gerador de carga (hashing repetido), útil para validar comportamento quando o próprio k6 vira gargalo.

```bash
docker run --rm \
  --cpus="0.50" \
  --memory="256m" \
  appconfig-cache:k6 run /k6/scripts/cpu_bound_test.js \
  -e VUS=2 \
  -e DURATION=45s \
  -e ROUNDS_PER_ITERATION=15000
```

### Teste k6 IO-bound

`scripts/k6/io_bound_test.js` prioriza espera de rede/IO, com bateladas HTTP para `/v1/config`.

```bash
docker run --rm \
  --cpus="0.50" \
  --memory="256m" \
  --add-host=host.docker.internal:host-gateway \
  appconfig-cache:k6 run /k6/scripts/io_bound_test.js \
  -e TARGET_URL="http://host.docker.internal:8080/v1/config?application=<app_id>&environment=<env_id>&profile=<profile_id>" \
  -e X_API_KEY="<x_api_key_opcional>" \
  -e RATE=20 \
  -e PARALLEL_REQUESTS=4
```

> Dica: se o objetivo é estressar a aplicação (e não o k6), mantenha `cpu_bound_test` com `ROUNDS_PER_ITERATION` baixo e priorize `io_bound_test`.

## Build para Lambda

```bash
make build-lambda
```

Artefato gerado:

- `dist/lambda.zip`

## Observações de deploy

- Runtime recomendado: `provided.al2023`
- Handler: `bootstrap` (arquivo dentro do zip)
- Arquitetura do build atual: `amd64`
