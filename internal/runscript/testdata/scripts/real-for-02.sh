for arch in x64 arm64; do
  scripts/package-binary.sh "installers/repository-installer-$arch" "$arch" packages
done
