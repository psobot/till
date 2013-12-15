Till
----

by Peter Sobot (@psobot)

----

Till is a cache server for immutable, time-limited block storage. It provides a simple interface via HTTP to put and get binary blobs, and supports the following storage providers:

 - Redis (`redis`)
 - Local filesystem (`file`)
 - Other Till servers (`till`)
 - S3 (`s3`)
 - Rackspace Cloud Files (`rackspace`)
 
A single request to Till can query all (or just one) of these storage providers in sequence.

Till is meant for immutable, time-limited cache data. It is very much a work in progress and is currently only used by [the Wub Machine](http://the.wubmachine.com) to manage upload caches. It has **no authorization control** on its inputs - so should be firewalled on your internal network, or only served locally on one host.

`tilld`, the binary that imlpements the Till server, will bind to port `5632` on `127.0.0.1` by default. This port can be overridden via the config file (`/var/till/till.config.json`) or the command line option `--port`/`-p`.


`tilld` command line
---
    tilld
    


Methods
----

#### `GET /api/v1/<object_identifier>`
Get an object from the cache.

Headers:

  - `X-Till-Provider` (**optional**): A comma-separated list of provider names to fetch from, where each name is defined in the configuration. If not provided, providers are fetched from in the order that they are configured.
  
#### `GET /api/v1/object/<object_identifier>/url`
Get an object's location in the cache. Returns a queryable URL to S3, Cloud Files, or `till` itself. Useful if you don't want the object itself, but you want its location to pass to someone else.

Headers:

  - `X-Till-Provider` (**optional**): A comma-separated list of provider names to fetch from, where each name is defined in the configuration. If not provided, providers are fetched from in the order that they are configured.
  
#### `POST /api/v1/object/<object_identifier>`
Post an object to the cache.

Headers:

  - `X-Till-Lifespan`: A number of seconds from now to persist the object for. After this many seconds, the object is deleted from all caches.
    
#### `PUT /api/v1/object/<object_identifier>`
Update an object in the cache. If the `PUT` body is empty, only update its properties as specified by the headers of the request.

Headers:

  - `X-Till-Lifespan`: A number of seconds from now to persist the object for. After this many seconds, the object is deleted from all caches.
  
#### `POST /api/v1/server/<server_identifier>`
Notify a Till server of the existence of another Till server.
Upon receiving this request, a Till server will respond with a POST request to the sender. If this POST request is successful, the server will be registered in the receiver's server table. If the `X-Till-Broadcast` header is not set to `0`, the server will forward the request to other Till servers that it knows about.

Headers:

  - `X-Till-IP`: a reachable (i.e.: non-local) IP that can be used to contact the sender.
  - `X-Till-Port`: the port that the sender is running on.
  - `X-Till-Lifespan` (**optional**, default `60`): A number of seconds from now to persist the sending server for. After this many seconds, the sending Till server is forgotten about by the receiver.
  - `X-Till-Broadcast` (**optional**, default `1`): A boolean (`1` or `0`) that specifies if this request should be re-broadcast onto other known Till servers.
  
  
Configuration
----

`config.json` should look something like the following

    {
        "port": 12345,
        "bind": "127.0.0.1",
        "providers": [
            {
                "type": "redis",
                "name": "my_redis_instance",
                
                "host": "123.123.123.123",
                "port": "7777",
                "db":   "mydb",
                
                "maxsize": 1073741824,
                "maxitems": 10000
            },
            {
                "type": "file",
                "name": "local_filesystem",
                
                "path": "/var/cache/till",
                "maxsize": 1073741824,
                "maxitems": 10000
            },
            {
                "type": "till",
                "name": "cluster",
                                
                "request_types": [
                    "local_filesystem"
                ],
                "servers": [
                    "123.123.123.123"
                ]
            },
            {
                "type": "s3",
                "name": "my_preferred_bucket",
                
                "aws_user_name": "username",
                "aws_access_key_id": "key",
                "aws_secret_access_key": "key",
                "aws_s3_bucket": "com.example.mybucket",
                "aws_s3_path": "optional/path/"   
            },
            {
                "type": "rackspace",
                "name": "my_preferred_rackspace",
                
                "rackspace_user_name": "username",
                "rackspace_api_key": "key",
                "rackspace_region": "key"      
            }
        ]
    }
    
Notes about the Till configuration:

 - `maxsize` is given in bytes.
 - Each provider is checked in sequence. In this example configuration, a `till` request will be satisfied by checking:
     - The Redis server running on host `123.123.123.123:7777`, in db `mydb`.
     - The local filesystem, in `/var/cache/till`.
     - Other nearby Till servers, starting with `123.123.123.123`. If `123.123.123.123` knows about other Till servers, they will be queried as well - in order of their registration.
     - S3, in `com.example.mybucket`, with the given credentials.
     - Rackspace Cloud Files.
     