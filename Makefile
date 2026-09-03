# Thin wrappers around the commands in the README. Nothing here is required:
# every target is one line you could type yourself.

.PHONY: up down run build test tidy psql health app webtest smoke e2e

up:    ## start PostgreSQL
	docker compose up -d

down:  ## stop PostgreSQL (keeps the volume)
	docker compose down

run:   ## run the backend (applies migrations on startup)
	go run ./cmd/jobpulse

build:
	go build -o jobpulse ./cmd/jobpulse

test:
	go test ./...

tidy:
	go mod tidy

psql:  ## open a shell on the dev database
	docker compose exec postgres psql -U jobpulse -d jobpulse

health:
	curl -fsS localhost:8080/healthz && echo

webtest:
	cd web && npm test

app:   ## run the web app against the local backend
	cd web && VITE_JOBPULSE_API=http://localhost:8080 npm run dev

smoke: ## exercise every route against a running backend (default localhost:8091)
	bash scripts/smoke.sh $(API)

e2e:   ## the browser suite: needs a backend with no boards and the seeded corpus
	cd e2e && npm install && npx playwright test
