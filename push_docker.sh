# GOOS=linux GOARCH=amd64 go build -trimpath -mod=readonly -ldflags "-s -w" -o ech-wokers
docker login
docker buildx install
docker buildx create --name multi-platform-builder --use --append 2>/dev/null || docker buildx use multi-platform-builder
docker buildx use multi-platform-builder
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v8 -t 718114245/ew . --push
