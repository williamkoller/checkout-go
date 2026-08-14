# Estágio de build
FROM golang:1.26.5-alpine AS builder

# Instalar dependências do sistema
RUN apk add --no-cache git gcc musl-dev

# Definir diretório de trabalho
WORKDIR /app

# Copiar arquivos de dependências
COPY go.mod go.sum ./
RUN go mod download

# Copiar código fonte
COPY . .

# Build da aplicação
RUN CGO_ENABLED=1 GOOS=linux go build -a -o main .

# Estágio final
FROM alpine:latest

# Instalar dependências de runtime
RUN apk add --no-cache ca-certificates sqlite

# Criar diretório para dados
RUN mkdir -p /app/data

# Definir diretório de trabalho
WORKDIR /app

# Copiar binário do builder
COPY --from=builder /app/main .

# Copiar arquivos necessários (se existirem)
COPY --from=builder /app/carrinho.db* /app/data/

# Configurar variáveis de ambiente
ENV PORT=8080
ENV REDIS_ADDR=redis:6379
ENV GIN_MODE=release

# Expor porta
EXPOSE 8080

# Comando para executar a aplicação
CMD ["./main"]