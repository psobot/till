import time
import requests


class TillServer(object):
    def __init__(self, address, timeout=1.5):
        self.address = address
        if self.address.startswith("http://"):
            self.address = self.address.replace("http://", "")
        self.timeout = timeout

    def __gen_url(self, id):
        return "http://%s/api/v1/object/%s" % (self.address, id)

    def put(self, key, value, metadata=None, providers=None):
        """Put a file into the Till server.
        `value` must be a file-like, `metadata` must be None or a string."""
        headers = {
            "X-Till-Lifespan": "default",
        }
        if providers is not None:
            headers["X-Till-Providers"] = ",".join(providers)

        if metadata is not None:
            assert len(metadata) < 4096, "Metadata must be less than 4096b."
            assert "\n" not in metadata, "Metadata must not contain newlines."
            headers['X-Till-Metadata'] = metadata
        r = requests.post(
            self.__gen_url(key),
            data=value,
            headers=headers,
            timeout=self.timeout
        )
        r.raise_for_status()

    def get(self, key, providers=None):
        """Get a file from the Till server."""

        headers = {}

        if providers is not None:
            headers["X-Till-Providers"] = ",".join(providers)

        r = requests.get(
            self.__gen_url(key),
            headers=headers,
            timeout=self.timeout,
            stream=True
        )
        r.raise_for_status()
        return (r.raw, r.headers.get("X-Till-Metadata", None))

if __name__ == "__main__":
    t = TillServer("localhost:12345")
    for x in xrange(0, 1000):
        key = "objectnumero%d" % x
        data = "hello world number %d" % x
        metadata = "hello metadatums number %d" % x
        t.put(key, data, metadata)
        o, metadata_ = t.get(key)
        resp = o.read(1024)
        assert resp == data
        assert metadata == metadata_
    for x in xrange(0, 1000):
        try:
            key = "objectnumber%d" % x
            o, metadata = t.get(key)
            assert False, "Object should not have been found!"
        except requests.HTTPError as e:
            if e.response.status_code != 404:
                raise
    print "it went okay"
