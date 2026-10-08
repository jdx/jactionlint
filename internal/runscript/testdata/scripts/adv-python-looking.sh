import os
with open(os.environ["GITHUB_ENV"], "a") as f:
    f.write("A=1\n")
