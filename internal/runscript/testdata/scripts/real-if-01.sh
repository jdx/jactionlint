awk '/^## \[/{if(found) exit; found=1} found{print}' CHANGELOG.md > release-notes.md
