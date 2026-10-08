python3 - <<'PY'
import json
import os
import sys

needs = json.loads(os.environ["NEEDS_JSON"])
failed = False
for name, data in sorted(needs.items()):
    result = data.get("result", "unknown")
    if result == "success":
        print(f"::notice::{name}: {result}")
    else:
        print(f"::error::{name}: {result}")
        failed = True

if failed:
    print("One or more upstream jobs did not complete successfully.")
    sys.exit(1)

print("All CI jobs completed successfully.")
PY
