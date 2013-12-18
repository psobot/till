Till
----

by Peter Sobot (@psobot)

----

Till is a cache server for immutable, time-limited object storage. It provides a simple interface via HTTP to put and get binary blobs and up to 4kB of metadata. Till smoothes over the different interfaces for cloud object storage and provides you with a simple set of HTTP methods for accessing your cache.

Till could be useful for you if:
 
 - You need to access some very frequently used objects very quickly (< 5ms), others less so (< 50ms) and others even less so (< 500ms). Till can use Redis, local files, or S3 to serve your data, falling back seamlessly from one to the next if cache misses occur.
 - You want to provide fast (< 50ms) access to cached binary data from your web app, but your servers have small disks. Set up Till on your local machines, and use `n` GB of each disk to store your cache. (If the cache gets too full, the newer items remain on disk, while the older items can be pushed to S3 or Cloud Files.)
 - You want to write your web app to use a cloud object store like S3 or Rackspace Cloud Files, but you don't want to have to use the annoying libraries for each in your project. Use plain-old HTTP to talk to `tilld` instead, and configure one or more object stores to be queried.
 - You're moving data from S3 to Cloud Files (or vice versa) and want to keep your data online as you migrate. Use Till as your interface, and configure it to only write to Cloud Files, while reading from both S3 and Cloud Files. 


Till supports the following storage providers:

 - Redis (`redis`)
 - Local filesystem (`file`)
 - S3 (`s3`)
 - Rackspace Cloud Files (`rackspace`)
 - Other Till servers (`till`)
 
A single request to Till can query all (or just one) of these storage providers at the same time, returning the fastest result.

Till is used for **immutable, time-limited** cache data. It is recommended that the keys used to store objects are message digests of the objects themselves, as Till does not allow updates to existing objects in the cache.

Till is very much a work in progress and is currently only used by [the Wub Machine](http://the.wubmachine.com) to manage upload caches. It has **no authorization control** on its inputs - so should be firewalled on your internal network, or only served locally on one host.


`tilld`, the binary that imlpements the Till server, will bind to port `5632` on `127.0.0.1` by default. This port can be overridden via the config file (`/var/till/till.config.json`) or the command line option `--port`/`-p`.

TODO
---

Many things. Till is still an extremely alpha project.

 - GetURL methods internally
 - Optimizations
 - Per-provider default TTLs
 - The entire tilld-to-tilld propagation system
 - Distributing objects across tilld servers
 - `select`ing on multiple Get requests and cancelling them once the first one comes back


Methods
----

**NOTE**: All `object_identifier`s below must conform to the regex `[a-zA-Z0-9_-.]+`.

#### `GET /api/v1/<object_identifier>`
Get an object from the cache.

Request Headers:

  - `X-Till-Provider` (**optional**): A comma-separated list of provider names to fetch from, where each name is defined in the configuration. If not provided, providers are fetched from simultaneously.
  - `X-Till-Lifespan` (**optional**): A number of seconds from now (or `default`) to persist the object for. After this many seconds, the object may be unavailable. Supplying this parameter is equivalent to issuing this `GET` request, immediately followed by a `PUT`.
  
Response Headers:

  - `X-Till-Metadata` (**optional**): A printable-ASCII string, up to 4096 bytes long and containing no newlines, that was stored along with the object. This header may be omitted if the object has no metadata.
  
Return codes:

  - `200 OK` is returned if an object with the given `object_identifer` exists in the cache somewhere.
  - `400 Bad Request` is returned if:
      - The supplied `X-Till-Lifespan` header is not a positive number or `default`.
      - The supplied `X-Till-Provider` header contains a provider name more than once.
  - `404 Not Found` is returned if no object with the given `object_identifier` could be found in the cache and all providers were checked.
  - `502 Bad Gateway` is returned if no object with the given `object_identifier` could be found and one or more providers failed to be queried.
  - `504 Gateway Timeout` is returned if no object with the given `object_identifier` could be found and one or more providers timed out during the request.
  
If a `5xx` error code is returned, the body of the response will be a JSON-encoded error message of the failed providers, like so:

    [
        "my_redis_instance": {"status": "OK"},
        "cluster": {"status": "TIMEOUT", "timeout_ms": 5000},
        "my_s3_bucket": {"status": "FAILURE", "error": "provider-specific error string"}
    ]
  
#### `GET /api/v1/object/<object_identifier>/url`
Get an object's location in the cache. Returns a queryable URL to S3, Cloud Files, or `till` itself. Useful if you don't want the object itself, but you want its location to pass to someone else.

Request Headers:

  - `X-Till-Providers` (**optional**): A comma-separated list of provider names to fetch from, where each name is defined in the configuration. If not provided, providers are fetched from in the order that they are configured.

Return codes:

  - `200 OK` is returned if an object with the given `object_identifer` exists in the cache somewhere. The body of the request contains a queryable URL.
  - `400 Bad Request` is returned if:
      - The supplied `X-Till-Provider` header contains a provider name more than once.

    In case of a bad request, the reason for the bad request will be supplied in quoted     plaintext (which happens to be valid JSON).

#### `POST /api/v1/object/<object_identifier>`
Add an object to the cache.

Request Headers:

  - `X-Till-Lifespan`: A number of seconds from now (or `default`) to persist the object for. After this many seconds, the object may be unavailable.
  - `X-Till-Synchronized` (**optional**, default `0`): A boolean (`1` or `0`) that specifies if this request should wait for acknowledgement of a write from all cache providers. If `0`, a response is returned once one cache provider acknowledges a successful write.
Response Headers:
  - `X-Till-Metadata` (**optional**): A printable-ASCII string, up to 4096 bytes long and containing no newlines, to be stored along with the object. This header may be omitted if the object has no metadata.
  - `X-Till-Providers` (**optional**): A comma-separated list of provider names to persist to, where each name is defined in the configuration. If not provided, providers are persisted to as per their configuration.  
  
Return codes:

  - `200 OK` is returned if an object with the given `object_identifer` already exists in the cache. If the object supplied by the request differs from the object already in the cache, **the object in the cache will remain** and the newly `POST`ed object will be ignored. (The `X-Till-Lifespan` header will be updated; the `X-Till-Metadata` value will not.)
  - `201 Created` is returned if the object has been persisted to all caches.
  - `202 Accepted` is returned if the object has been persisted to at least one cache.
  - `400 Bad Request` is returned if:
      - The request is missing an `X-Till-Lifespan` header.
      - The supplied `X-Till-Lifespan` header is not a positive number or `default`.
      - The supplied `X-Till-Synchronized` header is not exactly `1` or `0`.
      - The supplied `X-Till-Metadata` header is longer than 4096 bytes.
      
    In case of a bad request, the reason for the bad request will be supplied in quoted plaintext (which happens to be valid JSON).
  - `502 Bad Gateway` is returned if the object could not be persisted to any caches.
  - `504 Gateway Timeout` is returned if the object could not be persisted to any caches before they timed out.
    
#### `PUT /api/v1/object/<object_identifier>`
Update an object's lifespan in the cache. The body of this request must be empty, and the data to be updated must be specified by the headers of the request.

Request Headers:

  - `X-Till-Lifespan`: A number of seconds from now (or `default`) to persist the object for. After this many seconds, the object may be unavailable.  
  - `X-Till-Synchronized` (**optional**, default `0`): A boolean (`1` or `0`) that specifies if this request should wait for acknowledgement of a write from all cache providers. If `0`, a response is returned once one cache provider acknowledges a successful write.

  
Return codes:

  - `200 OK` is returned if an object with the given `object_identifer` exists in the cache and its lifespan was updated in all caches.
  - `202 Accepted` is returned if the object's new lifespan has been persisted to at least one cache.
  - `404 Not Found` is returned if the object could not be found in any cache.
  - `400 Bad Request` is returned if:
      - The request is missing an `X-Till-Lifespan` header.
      - The supplied `X-Till-Lifespan` header is not a positive number or `default`.
      - The supplied `X-Till-Synchronized` header is not exactly `1` or `0`.
      
    In case of a bad request, the reason for the bad request will be supplied in quoted plaintext (which happens to be valid JSON).


Internal Server Methods
---
  
#### `POST /api/v1/server/<server_identifier>`
Notify a Till server of the existence of another Till server.
Upon receiving this request, a Till server will respond with a POST request to the sender. If this POST request is successful, the server will be registered in the receiver's server table.

Request Headers:

  - `X-Till-Address`: a reachable (i.e.: non-local) IP/Port pair that can be used to contact the sender.
  - `X-Till-Lifespan` (**optional**, default `86400`): A number of seconds from now to persist the sending server for. After this many seconds, the sending Till server is forgotten about by the receiver. Once this time limit is up, the receiver sends a POST request back to the sender,  which prompts the 
  
  
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
                
                "aws_access_key_id": "key",
                "aws_secret_access_key": "key",
                "aws_s3_bucket": "com.example.mybucket",
                "aws_s3_path": "optional/path/",
                "aws_s3_storage_class": "REDUCED_REDUNDANCY"
            },
            {
                "type": "rackspace",
                "name": "my_preferred_rackspace",
                
                "rackspace_user_name": "username",
                "rackspace_api_key": "key",
                "rackspace_region": "key",
                "rackspace_path": "optional/path/"
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
     
Providers
---

###Redis

The Redis provider allows a bounded number of files to be cached in a Redis database. (Size-bounded Redis storage is currently not implemented.) The Redis provider has a number of unique properties:

 - When the item limit is reached and a new item is added to the cache, the Redis provider will expire an item at random to make room.
 
###Filesystem

The filesystem provider allows for a bounded number (or size) of files to be cached on a mounted filesystem at a given path. Metadata and expiry information is stored in JSON format in a separate `metadata` folder within the given path, while the object data itself is stored within a `files` folder.

###S3

The S3 provider allows for an unbounded number of files to be cached in Amazon S3. As S3 only allows for item expiration on a per-bucket basis, rather than a per-item basis, the `X-Till-Lifespan` header does not have any effect on an S3 provider. Instead, the item expiration **must be set manually** on the S3 bucket used with Till - otherwise, the cached items will remain indefinitely.

###Rackspace

The S3 provider allows for an unbounded number of files to be cached in Rackspace Cloud Files.
     