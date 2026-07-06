# Guia de Contribuição

Bem-vindo ao projeto `appconfig-cache`! Este documento orienta você sobre a configuração do seu ambiente de desenvolvimento local, validação de qualidade de código, envio de commits e adesão às nossas diretrizes de arquitetura de software.

---

## 🛠️ Configuração do Ambiente Local

Para garantir a consistência do código em todas as contribuições, usamos o **Lefthook** para gerenciamento de Git hooks e o **goimports** para organização das importações.

1. **Verifique se o Go está instalado:** Recomenda-se a versão `1.26+`.
2. **Instale as ferramentas e ative os Git Hooks:**
   Execute o seguinte comando na raiz do repositório:
   ```bash
   make setup
   ```
   Esta tarefa automaticamente:
   - Baixa e instala o `lefthook` e o `goimports` localmente no diretório `GOPATH/bin`.
   - Inicializa os Git hooks para pre-commit (formatação, imports, linting) e commit-msg (validação de commits convencionais).

---

## 🔄 Fluxo de Desenvolvimento e Verificação Local

Antes de abrir um Pull Request, execute a suíte de verificação local para garantir que todas as validações passem:

### 1. Formatador e Linter
- **Formatar código:** `go fmt ./...`
- **Organizar importações:** `$(go env GOPATH)/bin/goimports -w -local github.com/sousapedro11/appconfig-cache .`
- **Linter Estático:** `golangci-lint run`

### 2. Testes Unitários
Todos os testes neste repositório são **100% herméticos**: eles rodam inteiramente em memória, sem dependência de instâncias reais do Valkey (Redis) ou serviços da AWS. Os mocks e stubs são injetados no nível do pacote.
- **Executar testes e medir cobertura:**
  ```bash
  go test -race -coverprofile=coverage.out -covermode=atomic ./... && go tool cover -func=coverage.out
  ```

---

## 💬 Padrão de Commits Convencionais (Conventional Commits)

Cada mensagem de commit neste projeto é verificada pelo hook `commit-msg` para garantir que segue a especificação de **Conventional Commits**:

```text
<tipo>(<escopo opcional>): <verbo no imperativo> <descrição>
```

### Tipos Permitidos
- **`feat`**: Uma nova funcionalidade.
- **`fix`**: Correção de bug.
- **`refactor`**: Alterações de código que não corrigem bugs nem adicionam funcionalidades.
- **`docs`**: Atualizações de documentação.
- **`test`**: Adição de testes ausentes ou correção de testes existentes.
- **`chore`**: Manutenção, arquivos de build, dependências ou configurações de ferramentas (ex: atualizações do Lefthook).
- **`build`**: Alterações que afetam o sistema de build ou pacotes externos.
- **`ci`**: Atualizações de arquivos de CI (ex: workflows do GitHub Actions).
- **`style`**: Alterações que não afetam o significado do código (formatação, espaços em branco, organização de imports).

*Exemplos:*
- `feat(cmd): implement Standalone HTTP server`
- `style: format imports and group dependencies`
- `chore(lefthook): fix sequential validation runner`

---

## 🎨 Convenções de Código de Software

Nossa base de código foi projetada para ser minimalista, limpa e altamente robusta. Por favor, siga estas convenções:

1. **Object Calisthenics & Clean Code:**
   - **Proibido usar a palavra-chave `else`:** Minimize o aninhamento e escreva uma lógica mais linear e limpa usando cláusulas de guarda e retornos antecipados.
   - **Apenas um nível de indentação por método:** Mantenha funções pequenas e com uma única responsabilidade.
2. **Tipagem Forte:**
   - Evite o uso de interfaces vazias (`interface{}` ou `any`), a menos que seja estritamente necessário para payloads genéricos ou serializações.
3. **YAGNI & Regra de Comentários:**
   - Sempre priorize a simplicidade. Evite sobre-engenharia ou preparar o código para futuros casos de uso não solicitados.
   - **Justificativa de decisões:** Use anotações de comentários para justificar decisões de design minimalistas, otimizadas ou baseadas em YAGNI.
4. **Testes Herméticos e Rápidos:**
   - Os testes unitários devem rodar instantaneamente. Evite subir containers docker ou escutar portas de sockets reais nos testes. Use mocks de interfaces padrão e stubs de funções locais.
