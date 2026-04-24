# Pipeline State

## Config
feature: {{.Feature}}
mode: {{.Mode}}
project_mode: {{.ProjectMode}}
branch: {{.Branch}}

## Repo Map
| Repo | Path | Lang | Specialist | Domain Planner |
|------|------|------|------------|----------------|
{{- range .RepoMap }}
| {{.Name}} | {{.Path}} | {{.Lang}} | {{.Specialist}} | {{.DomainPlanner}} |
{{- end }}

## Env
playwright_skill: {{.PlaywrightSkill}}
test_auth_missing: {{.TestAuthMissing}}

## Phase Status
| Phase | Status | Artifact | Notes |
|-------|--------|----------|-------|
{{- range .Phases }}
| {{.Phase}} | {{.Status}} | {{.Artifact}} | {{.Notes}} |
{{- end }}

## Execution Order
{{.ExecutionOrder}}

## Results (per repo)
{{.Results}}
