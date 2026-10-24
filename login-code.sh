#!/bin/bash
# Prints the most recent Dakiya Mail login code from Mailpit.
ID=$(curl -s -m 10 "http://localhost:18025/api/v1/messages?limit=1" \
  | python3 -c "import json,sys; m=json.load(sys.stdin).get('messages',[]); print(m[0]['ID'] if m else '')")
[ -z "$ID" ] && { echo "No email yet — submit the password first."; exit 1; }
curl -s -m 10 "http://localhost:18025/api/v1/message/$ID" \
  | python3 -c "
import json,sys,re
d=json.load(sys.stdin)
c=re.findall(r'\b\d{6}\b', d.get('Text') or '')
print('Login code:', c[0] if c else 'not found')
"
