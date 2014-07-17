Dockerfiles
===========

As this project depends so critically on NSQ, we maintain a set of dockerfiles
to NSQ up and running. These are also up on the docker hub in mike's account.
See the [mikedewar/nsq](https://registry.hub.docker.com/u/mikedewar/nsq/) for
more. 

Run

```
docker run -d  --name=nsqlookupd -e BROADCAST_ADDRESS=192.168.59.103 -p
4160:4160 -p 4161:4161 mikedewar/nsqlookupd
docker run -d --name=nsqd -e BROADCAST_ADDRESS=192.168.59.103 -p 4150:4150 -p
4151:4151 --link nsqlookupd:nsqlookupd mikedewar/nsqd
docker run -d  --name=nsqadmin -p 4171:4171 --link nsqlookupd:nsqlookupd
mikedewar/nsqadmin
```

to quickly get an NSQ setup working using docker. 
