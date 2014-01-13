#!/usr/bin/env python

import os
import sys
import json
import time
import base64
import socket
import random
import requests
import traceback
import contextlib
import subprocess
from colorama import Fore
from subprocess import Popen


def make_obj_url(address, port, ob):
    return "http://" + address + ":" + port + "/api/v1/object/" + ob


def good(string):
    print Fore.GREEN + string + Fore.RESET


def bad(string):
    print Fore.RED + string + Fore.RESET


def unknown(string):
    print Fore.YELLOW + string + Fore.RESET


def randport():
    return int((random.random() * ((2 ** 16) - 1024)) + 1024)


@contextlib.contextmanager
def redis_server(port=None):
    if port is None:
        port = randport()

    good("Starting redis server on port %d..." % port)
    procs = []
    try:
        procs = [Popen(
            ['redis-server', '--port', str(port), '--save', "0"],
            stdout=open('/dev/null', 'w'),
            stderr=open('/dev/null', 'w'),
        )]
        yield
    finally:
        good("Killing redis server.")
        for proc in procs:
            if proc and proc.poll() is None:
                proc.kill()


def gen_single_config(port, redis_port):
    return {
        "port": port,
        "bind": "127.0.0.1",
        "public_address": "127.0.0.1:%d" % port,
        "default_lifespan": 3600,
        "providers": [
            {
                "type": "redis",
                "name": "test_redis",
                "whitelist": [".*"],

                "host": "localhost",
                "port": redis_port,
                "db": 0,
                "maxitems": 50,
            },
            {
                "type": "file",
                "name": "test_file",
                "whitelist": [".*"],

                "path": "/tmp/till_%d" % port,
                "maxsize": 1024 * 1024,

                "maxitems": 10,
            }
        ]
    }

SINGLE_PROVIDER_NAMES = [
    "test_redis",
    "test_file",
]


def gen_multiple_config(port, cluster_port):
    return {
        "port": port,
        "bind": "127.0.0.1",
        "public_address": "127.0.0.1:%d" % port,
        "default_lifespan": 3600,
        "providers": [
            {
                "type": "file",
                "name": "test_file",
                "whitelist": [".*"],

                "path": "/tmp/till_%d" % port,
                "maxsize": 1024 * 1024,

                "maxitems": 10,
            },
            {
                "type": "till",
                "name": "test_cluster",
                "whitelist": [".*"],

                "request_types": ["file", "redis"],
                "servers": ["127.0.0.1:%d" % cluster_port],
            }
        ]
    }


MULTIPLE_PROVIDER_NAMES = [
    "test_redis",
    "test_file",
    "test_cluster",
]


def test(*funcs):
    good("================= STARTING TEST ===============")
    procs = []
    try:
        good("Launching tilld.")
        tilld_port = randport()
        redis_port = randport()
        udp_recv = randport()
        env = {
            "TEST_UDP_PORT": str(udp_recv),
            "TILL_CONFIG":
            json.dumps(gen_single_config(tilld_port, redis_port))
        }
        env = dict(os.environ.items() + env.items())
        with redis_server(redis_port):
            procs = [Popen(['./bin/tilld'], env=env)]

            address, port = "localhost", str(tilld_port)

            unknown("Waiting for launch.")
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            sock.bind(("127.0.0.1", udp_recv))
            sock.recvfrom(1)
            sock.close()
            good("Tilld launch detected. Running tests.")

            for func in funcs:
                a = time.time()
                try:
                    success, code = func(address, port)
                except Exception as e:
                    traceback.print_exc()
                    success, code = False, e
                b = time.time()
                if success is True:
                    good("[x] Test %s complete! (%2.2f msec)"
                         % (func.__name__, (b - a) * 1000.0))
                else:
                    bad("[ ] Test %s failed! (%2.2f msec, received %s)"
                        % (func.__name__, (b - a) * 1000.0, code))
    finally:
        for proc in procs:
            if proc and proc.poll() is None:
                proc.kill()
        try:
            subprocess.call(["rm", "-rf", "/tmp/till_%d" % redis_port])
        except Exception:
            pass


def cluster_test_master(*args, **kwargs):
    return cluster_test(*args, query_master=True)


def cluster_test_slave(*args, **kwargs):
    return cluster_test(*args, query_master=False)


def cluster_test_both(*args, **kwargs):
    return cluster_test(*args, query_master=False, both=True, delay=1000)


def cluster_test(*funcs, **kwargs):
    query_master = kwargs.get('query_master', False)
    both = kwargs.get('both', False)
    delay = kwargs.get('delay', 0)

    good("================= STARTING CLUSTER TEST ===============")
    procs = []
    try:
        good("Launching tilld the first.")
        tilld_port = randport()
        redis_port = randport()
        udp_recv = randport()
        env = {
            "TEST_UDP_PORT": str(udp_recv),
            "TILL_CONFIG":
            json.dumps(gen_single_config(tilld_port, redis_port))
        }
        env = dict(os.environ.items() + env.items())
        with redis_server(redis_port):
            procs = [Popen(['./bin/tilld'], env=env)]

            address, port = "localhost", str(tilld_port)

            unknown("Waiting for launch notification on UDP port %d." % udp_recv)
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            sock.bind(("127.0.0.1", udp_recv))
            sock.recvfrom(1)
            sock.close()

            good("Launching tilld the second.")
            tilld_port_2 = randport()
            udp_recv = randport()
            env = {
                "TEST_UDP_PORT": str(udp_recv),
                "TILL_CONFIG":
                json.dumps(gen_multiple_config(tilld_port_2, tilld_port))
            }
            env = dict(os.environ.items() + env.items())
            procs += [Popen(['./bin/tilld'], env=env)]

            address, port = "localhost", str(tilld_port)

            unknown("Waiting for launch notification on UDP port %d." % udp_recv)
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            sock.bind(("127.0.0.1", udp_recv))
            sock.recvfrom(1)
            sock.close()

            if delay > 0:
                good("Tilld launch detected. Running tests in %2.2f msec." % delay)
                time.sleep(delay / 1000.0)
            else:
                good("Tilld launch detected. Running tests.")

            for func in funcs:
                a = time.time()
                try:
                    if both:
                        success, code = func(
                            address,
                            str(tilld_port),
                            str(tilld_port_2)
                        )
                    else:
                        success, code = func(
                            address,
                            str(tilld_port) if query_master
                            else str(tilld_port_2)
                        )
                except Exception as e:
                    traceback.print_exc()
                    success, code = False, e
                b = time.time()
                if success is True:
                    good("[x] Test %s complete! (%2.2f msec)"
                         % (func.__name__, (b - a) * 1000.0))
                else:
                    bad("[ ] Test %s failed! (%2.2f msec, received %s)"
                        % (func.__name__, (b - a) * 1000.0, code))
    finally:
        for proc in procs:
            if proc and proc.poll() is None:
                proc.kill()
        try:
            subprocess.call(["rm", "-rf", "/tmp/till_%d" % redis_port])
        except Exception:
            pass


def post_no_headers(address, port):
    headers = {}
    obj_name = sys._getframe().f_code.co_name
    r = requests.post(make_obj_url(address, port, obj_name), headers=headers)
    return r.status_code == 400, r.status_code


def post_invalid_lifespan(address, port):
    headers = {"X-Till-Lifespan": "-12"}
    obj_name = sys._getframe().f_code.co_name
    r = requests.post(make_obj_url(address, port, obj_name), headers=headers)
    return r.status_code == 400, r.status_code


def post_invalid_lifespan_2(address, port):
    headers = {"X-Till-Lifespan": "ascii"}
    obj_name = sys._getframe().f_code.co_name
    r = requests.post(make_obj_url(address, port, obj_name), headers=headers)
    return r.status_code == 400, r.status_code


def post_default_lifespan(address, port):
    headers = {"X-Till-Lifespan": "default"}
    obj_name = sys._getframe().f_code.co_name
    r = requests.post(make_obj_url(address, port, obj_name), headers=headers)
    return r.status_code == 202, r.status_code


def post_invalid_synchronized(address, port):
    headers = {"X-Till-Synchronized": "2"}
    obj_name = sys._getframe().f_code.co_name
    r = requests.post(make_obj_url(address, port, obj_name), headers=headers)
    return r.status_code == 400, r.status_code


def post_invalid_synchronized_2(address, port):
    headers = {"X-Till-Synchronized": "true"}
    obj_name = sys._getframe().f_code.co_name
    r = requests.post(make_obj_url(address, port, obj_name), headers=headers)
    return r.status_code == 400, r.status_code


def post_too_long_metadata(address, port):
    headers = {"X-Till-Metadata": "x" * 4097}
    obj_name = sys._getframe().f_code.co_name
    r = requests.post(make_obj_url(address, port, obj_name), headers=headers)
    return r.status_code == 400, r.status_code


def post_ok_metadata(address, port):
    headers = {"X-Till-Metadata": "x" * 4095}
    obj_name = sys._getframe().f_code.co_name
    r = requests.post(make_obj_url(address, port, obj_name), headers=headers)
    return r.status_code == 400, r.status_code


def post_case_sensitive_lifespan(address, port):
    headers = {"x-till-lifespan": "123"}
    obj_name = sys._getframe().f_code.co_name
    r = requests.post(make_obj_url(address, port, obj_name), headers=headers)
    return r.status_code == 202, r.status_code


def post_with_lifespan(address, port):
    headers = {"X-Till-Lifespan": "123"}
    obj_name = sys._getframe().f_code.co_name
    r = requests.post(make_obj_url(address, port, obj_name), headers=headers)
    return r.status_code == 202, r.status_code


def post_synchronous(address, port):
    headers = {"X-Till-Lifespan": "10", "X-Till-Synchronized": "1"}
    obj_name = sys._getframe().f_code.co_name
    r = requests.post(make_obj_url(address, port, obj_name), headers=headers)
    return r.status_code == 201, r.status_code


def post_get_all(address, port):
    #   Post a file to all caches, synchronized, then get it back.
    metadata = "\n".join(['meta data'] * 100)
    headers = {
        "X-Till-Lifespan": "default",
        "X-Till-Synchronized": "1",
        "X-Till-Metadata": base64.encodestring(metadata).replace("\n", "")
    }
    obj_name = sys._getframe().f_code.co_name
    url = make_obj_url(address, port, obj_name)
    data = "\n".join(['test data'] * 100)
    r = requests.post(url, data=data, headers=headers)
    if r.status_code != 201:
        #   Make sure there is a JSON error message returned.
        assert r.json(), str(r.json())
        return False, r.status_code

    #   Don't send headers again here.
    r = requests.get(url)
    assert r.text == data
    assert base64.decodestring(r.headers.get("X-Till-Metadata")) == metadata
    return True, r.status_code


def post_get_wrong(address, port):
    #   Post a file to the first cache, try to get it from another, and fail.
    metadata = "\n".join(['meta data'] * 100)
    headers = {
        "X-Till-Lifespan": "default",
        "X-Till-Synchronized": "1",
        "X-Till-Metadata": base64.encodestring(metadata).replace("\n", ""),
        "X-Till-Providers": ",".join(SINGLE_PROVIDER_NAMES[0:1]),
    }
    obj_name = sys._getframe().f_code.co_name
    url = make_obj_url(address, port, obj_name)
    data = "\n".join(['test data'] * 100)
    r = requests.post(url, data=data, headers=headers)
    if r.status_code != 201:
        #   Make sure there is a JSON error message returned.
        assert r.json(), str(r.json())
        return False, r.status_code

    #   Don't send headers again here.
    r = requests.get(url, headers={
        "X-Till-Providers": ",".join(SINGLE_PROVIDER_NAMES[1:2]),
    })
    return r.status_code == 404, r.status_code


def post_get_correct(address, port):
    #   Post a file to the first cache, try to get it from another, and fail.
    metadata = "\n".join(['meta data'] * 100)
    headers = {
        "X-Till-Lifespan": "default",
        "X-Till-Synchronized": "1",
        "X-Till-Metadata": base64.encodestring(metadata).replace("\n", ""),
        "X-Till-Providers": ",".join(SINGLE_PROVIDER_NAMES[1:2]),
    }
    obj_name = sys._getframe().f_code.co_name
    url = make_obj_url(address, port, obj_name)
    data = "\n".join(['test data'] * 100)
    r = requests.post(url, data=data, headers=headers)
    if r.status_code != 201:
        #   Make sure there is a JSON error message returned.
        assert r.json(), str(r.json())
        return False, r.status_code

    #   Don't send headers again here.
    r = requests.get(url, headers={
        "X-Till-Providers": ",".join(SINGLE_PROVIDER_NAMES[1:2]),
    })
    assert r.text == data
    return r.status_code == 200, r.status_code


def post_get_scatter(address, port):
    #   Post a file to the first cache, try to get it from another, and fail.
    metadata = "\n".join(['meta data'] * 100)
    headers = {
        "X-Till-Lifespan": "default",
        "X-Till-Synchronized": "1",
        "X-Till-Metadata": base64.encodestring(metadata).replace("\n", ""),
        "X-Till-Providers": ",".join(SINGLE_PROVIDER_NAMES[1:2]),
    }
    obj_name = sys._getframe().f_code.co_name
    url = make_obj_url(address, port, obj_name)
    data = "\n".join(['test data'] * 100)
    r = requests.post(url, data=data, headers=headers)
    if r.status_code != 201:
        #   Make sure there is a JSON error message returned.
        assert r.json(), str(r.json())
        return False, r.status_code

    #   Don't send headers again here.
    r = requests.get(url)
    assert r.text == data
    return r.status_code == 200, r.status_code


def post_get_cluster(address, port1, port2):
    #   Post a file to the slave, try to get it from the master, and pass.
    metadata = "\n".join(['meta data'] * 100)
    headers = {
        "X-Till-Lifespan": "default",
        "X-Till-Synchronized": "1",
        "X-Till-Metadata": base64.encodestring(metadata).replace("\n", ""),
    }
    obj_name = sys._getframe().f_code.co_name
    url1 = make_obj_url(address, port1, obj_name)
    data = "\n".join(['test data'] * 100)
    r = requests.post(url1, data=data, headers=headers)
    if r.status_code != 201:
        #   Make sure there is a JSON error message returned.
        print repr(r.text)
        assert r.json(), str(r.json())
        return False, r.status_code

    #   Don't send headers again here.
    url2 = make_obj_url(address, port2, obj_name)
    r = requests.get(url2)
    return r.status_code == 200 and r.text == data, r.status_code


if __name__ == "__main__":
    unknown("Launching test cases...")
    unknown("Press Ctrl-C to stop the tests.")
    test(
        post_no_headers,
        post_no_headers,
        post_invalid_lifespan,
        post_invalid_lifespan_2,
        post_default_lifespan,
        post_invalid_synchronized,
        post_invalid_synchronized_2,
        post_case_sensitive_lifespan,
        post_with_lifespan,
        post_synchronous,
        post_get_all,
        post_get_wrong,
        post_get_correct,
        post_get_scatter,
    )
    cluster_test_master(
        post_no_headers,
        post_no_headers,
        post_invalid_lifespan,
        post_invalid_lifespan_2,
        post_default_lifespan,
        post_invalid_synchronized,
        post_invalid_synchronized_2,
        post_case_sensitive_lifespan,
        post_with_lifespan,
        post_synchronous,
        post_get_all,
        post_get_wrong,
        post_get_correct,
        post_get_scatter,
    )
    cluster_test_slave(
        post_no_headers,
        post_no_headers,
        post_invalid_lifespan,
        post_invalid_lifespan_2,
        post_default_lifespan,
        post_invalid_synchronized,
        post_invalid_synchronized_2,
        post_case_sensitive_lifespan,
        post_with_lifespan,
        post_synchronous,
    )
    cluster_test_both(
        post_get_cluster,
    )
