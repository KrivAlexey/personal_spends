terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

variable "aws_region" {
  description = "AWS region for all resources"
  type        = string
  default     = "eu-central-1"
}

# Single-table design — see docs/architecture.md for the full data model.
# PK is always "EXPENSES"; SK is "<YYYY-MM-DD>#<uuid>" for date-range queries.
# GSI1 (GSI1PK="CAT#<category>", GSI1SK=SK) supports category queries.
resource "aws_dynamodb_table" "expenses" {
  name         = "personal-spends-expenses"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "PK"
  range_key    = "SK"

  attribute {
    name = "PK"
    type = "S"
  }

  attribute {
    name = "SK"
    type = "S"
  }

  attribute {
    name = "GSI1PK"
    type = "S"
  }

  attribute {
    name = "GSI1SK"
    type = "S"
  }

  global_secondary_index {
    name            = "GSI1"
    hash_key        = "GSI1PK"
    range_key       = "GSI1SK"
    projection_type = "ALL"
  }

  tags = {
    Project = "personal-spends"
  }
}

output "expenses_table_name" {
  value = aws_dynamodb_table.expenses.name
}

output "expenses_table_arn" {
  value = aws_dynamodb_table.expenses.arn
}
