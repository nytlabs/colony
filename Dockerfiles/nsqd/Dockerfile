FROM mikedewar/nsq

EXPOSE 4150
EXPOSE 4151
ENTRYPOINT nsqd --lookupd-tcp-address=$NSQLOOKUPD_ADDRESS:4160 --broadcast-address=$BROADCAST_ADDRESS
