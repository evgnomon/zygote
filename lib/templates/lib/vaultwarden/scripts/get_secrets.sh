#!/usr/bin/env bash
python client.py --url https://localhost:8084 --email hg@evgnomon.org --json --pass-file <(getsecret | jq -r '.vaultwarden.local.pass')
