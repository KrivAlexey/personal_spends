# /deploy

Runs `terraform apply` in the `terraform/` directory after showing a plan summary.

Always confirm the plan output before proceeding. Never auto-approve.

```bash
cd terraform && terraform plan && terraform apply
```
