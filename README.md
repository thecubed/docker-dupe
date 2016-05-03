#Docker Dupe
This app allows you to copy docker layers and manifests from one docker registry server to another directly.

Normally, you'd have to do something like:
```
docker pull my.registry.com/my-image:latest
docker tag my.registry.com/my-image:latest newregistry.company.com/my-image:latest
docker push newregistry.company.com/my-image:latest
```
This will take a long time, consume a lot of local disk space, and it requires your intermediate host be running docker at all.

Docker Dupe copies directly from registry to registry with no local storage necessary.

Manifests are kept unmodified, and signed with their original signature to preserve chain of trust.

## How to use
Simply run docker-dupe with the parameters you wish to use, and off it'll go:
```
docker-dupe -s http://my.registry.com -d http://newregistry.company.com -n my-image -t latest -c 4
```

### Command-line arguments
```
Usage:
  docker-dupe [OPTIONS]

Application Options:
      --debug        Enable DEBUG logging
  -V, --version      Print version and exit
  -s, --source=      Source docker registry URL
  -d, --dest=        Destination docker registry URL
  -n, --name=        Docker manifest name to pull from source
  -t, --tag=         Docker manifest tag to pull from source
  -c, --concurrency= Concurrent operation limit (default: 4)

Help Options:
  -h, --help         Show this help message
```

