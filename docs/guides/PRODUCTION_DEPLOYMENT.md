# Production Deployment Guide

> **Deploy Relica Applications to Production**
> **Version**: v0.5.0

---

## Pre-Deployment Checklist

- [ ] All tests passing
- [ ] Migrations tested on staging
- [ ] Connection pool configured
- [ ] Security features enabled (validator, auditor)
- [ ] Error handling comprehensive
- [ ] Logging configured
- [ ] Health checks implemented
- [ ] Monitoring in place

---

## Database Configuration

### Production Settings

```go
db, err := relica.Open("postgres", dsn,
    // Connection pool
    relica.WithMaxOpenConns(25),     // 10-25 per CPU core
    relica.WithMaxIdleConns(5),      // 20% of MaxOpenConns
    relica.WithConnMaxLifetime(300), // 5 minutes

    // Security
    relica.WithValidator(validator),  // SQL injection prevention
    relica.WithAuditLog(auditor),     // Compliance logging

    // Performance
    relica.WithStmtCacheCapacity(1000), // Prepared statement cache
)
```

### Environment Variables

```bash
# .env.production
DB_DRIVER=postgres
DB_HOST=db.example.com
DB_PORT=5432
DB_NAME=myapp_production
DB_USER=myapp
DB_PASSWORD=SecurePasswordHere
DB_SSL_MODE=require
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME=300
```

**Load in code:**
```go
dsn := fmt.Sprintf(
    "postgres://%s:%s@%s:%s/%s?sslmode=%s",
    os.Getenv("DB_USER"),
    os.Getenv("DB_PASSWORD"),
    os.Getenv("DB_HOST"),
    os.Getenv("DB_PORT"),
    os.Getenv("DB_NAME"),
    os.Getenv("DB_SSL_MODE"),
)
```

---

## Health Checks

### Basic Health Check

```go
func healthCheck(db *relica.DB) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    return db.PingContext(ctx)
}
```

### HTTP Health Endpoint

```go
func healthHandler(db *relica.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if err := db.PingContext(r.Context()); err != nil {
            w.WriteHeader(http.StatusServiceUnavailable)
            json.NewEncoder(w).Encode(map[string]string{
                "status": "unhealthy",
                "error":  err.Error(),
            })
            return
        }

        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "healthy",
        })
    }
}
```

---

## Migrations

### Using golang-migrate

```bash
# Install
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Create migration
migrate create -ext sql -dir migrations -seq create_users_table

# Run migrations
migrate -path migrations -database "postgres://user:pass@localhost/db" up
```

**migrations/000001_create_users_table.up.sql:**
```sql
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);
```

---

## Monitoring

### Metrics to Track

```go
type DBMetrics struct {
    OpenConnections  int
    InUseConnections int
    IdleConnections  int
    WaitCount        int64
    WaitDuration     time.Duration
}

func collectMetrics(db *relica.DB) DBMetrics {
    stats := db.Stats()
    return DBMetrics{
        OpenConnections:  stats.OpenConnections,
        InUseConnections: stats.InUse,
        IdleConnections:  stats.Idle,
        WaitCount:        stats.WaitCount,
        WaitDuration:     stats.WaitDuration,
    }
}
```

### Prometheus Integration

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    dbConnections = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "db_connections",
            Help: "Database connections",
        },
        []string{"state"},
    )
)

func updateMetrics(db *relica.DB) {
    stats := db.Stats()
    dbConnections.WithLabelValues("open").Set(float64(stats.OpenConnections))
    dbConnections.WithLabelValues("in_use").Set(float64(stats.InUse))
    dbConnections.WithLabelValues("idle").Set(float64(stats.Idle))
}
```

---

## Security

### Enable Validator

```go
import "github.com/coregx/relica/internal/security"

validator := security.NewValidator(security.WithStrictMode())
db, err := relica.Open("postgres", dsn,
    relica.WithValidator(validator),
)
```

### Enable Audit Logging

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
auditor := security.NewAuditor(logger, security.AuditReads)

db, err := relica.Open("postgres", dsn,
    relica.WithAuditLog(auditor),
)

// Add context metadata
ctx = security.WithUser(ctx, getUserFromToken(r))
ctx = security.WithClientIP(ctx, r.RemoteAddr)
ctx = security.WithRequestID(ctx, r.Header.Get("X-Request-ID"))
```

---

## Docker Deployment

### Dockerfile

```dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/api

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/server .

EXPOSE 8080
CMD ["./server"]
```

### docker-compose.yml

```yaml
version: '3.8'

services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=db
      - DB_PORT=5432
      - DB_USER=myapp
      - DB_PASSWORD=secret
      - DB_NAME=myapp_production
    depends_on:
      - db

  db:
    image: postgres:15
    environment:
      - POSTGRES_USER=myapp
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=myapp_production
    volumes:
      - pgdata:/var/lib/postgresql/data

volumes:
  pgdata:
```

---

## Kubernetes Deployment

### deployment.yaml

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  replicas: 3
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
      - name: myapp
        image: myapp:latest
        ports:
        - containerPort: 8080
        env:
        - name: DB_HOST
          valueFrom:
            secretKeyRef:
              name: db-secret
              key: host
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: db-secret
              key: password
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
```

---

## Graceful Shutdown

```go
func main() {
    db, err := setupDB()
    if err != nil {
        log.Fatal(err)
    }

    server := &http.Server{Addr: ":8080"}

    // Graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    go func() {
        <-sigChan
        log.Println("Shutting down...")

        ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()

        server.Shutdown(ctx)
        db.Close()
    }()

    log.Fatal(server.ListenAndServe())
}
```

---

## Backup and Recovery

### PostgreSQL Backup

```bash
# Automated backup
pg_dump -h localhost -U myapp myapp_production > backup_$(date +%Y%m%d).sql

# Restore
psql -h localhost -U myapp myapp_production < backup_20250113.sql
```

### Backup Strategy

- **Daily**: Full database backup
- **Hourly**: Incremental backups
- **Retention**: 7 days daily, 4 weeks weekly, 12 months monthly
- **Test restores**: Monthly

---

*For more details, see [Best Practices Guide](BEST_PRACTICES.md)*
