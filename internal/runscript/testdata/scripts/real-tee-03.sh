docker buildx imagetools inspect "$IMAGE"
docker run --rm --entrypoint /usr/local/bin/mise "$IMAGE" --version | tee /dev/stderr | grep -F "$VERSION"
