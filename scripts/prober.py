import datetime as dt
import http.cookiejar
import http.server
import json
import os
import threading
import time
import urllib.request
import uuid


APP_URL = os.getenv("APP_URL", "http://rooms-app:8080")
USERNAME = os.environ["PROBE_USERNAME"]
PASSWORD = os.environ["PROBE_PASSWORD"]

lock = threading.Lock()
cycles_total = 0
cycles_success_total = 0


def check_application():
    cookies = http.cookiejar.CookieJar()
    client = urllib.request.build_opener(
        urllib.request.HTTPCookieProcessor(cookies)
    )

    def request(method, path, expected_status, body=None):
        data = json.dumps(body).encode() if body is not None else None

        req = urllib.request.Request(
            APP_URL + path,
            data=data,
            method=method,
            headers={"Content-Type": "application/json"},
        )

        with client.open(req, timeout=5) as response:
            if response.status != expected_status:
                raise RuntimeError(
                    f"{path}: ожидался {expected_status}, "
                    f"получен {response.status}"
                )

            raw = response.read()
            return json.loads(raw) if raw else None

    room_id = None

    try:
        request("POST", "/api/login", 200, {
            "username": USERNAME,
            "password": PASSWORD,
        })

        room = request("POST", "/api/rooms", 201, {
            "name": "slo-probe-" + uuid.uuid4().hex,
            "capacity": 2,
            "location": "Проверка SLI",
        })
        room_id = room["id"]

        start = dt.datetime.now(dt.timezone.utc) + dt.timedelta(days=1)
        end = start + dt.timedelta(minutes=15)

        booking = request("POST", "/api/bookings", 201, {
            "roomId": room_id,
            "purpose": "Проверка SLI",
            "startsAt": start.isoformat(),
            "endsAt": end.isoformat(),
        })

        request("DELETE", f"/api/bookings/{booking['id']}", 204)
        return True

    except Exception as error:
        print(f"Проверка не пройдена: {error}", flush=True)
        return False

    finally:
        # Удаляем только созданную этим циклом тестовую комнату.
        # В исходной схеме связанные бронирования удаляются каскадно.
        if room_id is not None:
            try:
                request("DELETE", f"/api/rooms/{room_id}", 204)
            except Exception as error:
                print(
                    f"Не удалось удалить тестовую комнату {room_id}: {error}",
                    flush=True,
                )

        try:
            request("POST", "/api/logout", 204)
        except Exception as error:
            print(f"Ошибка выхода: {error}", flush=True)


def run_checks():
    global cycles_total, cycles_success_total

    while True:
        started = time.monotonic()
        success = check_application()

        with lock:
            cycles_total += 1
            cycles_success_total += int(success)

        print(
            "Результат цикла: " + ("успех" if success else "ошибка"),
            flush=True,
        )

        time.sleep(max(0, 10 - (time.monotonic() - started)))


class MetricsHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/metrics":
            self.send_error(404)
            return

        with lock:
            body = (
                "# TYPE probe_cycles_total counter\n"
                f"probe_cycles_total {cycles_total}\n"
                "# TYPE probe_cycles_success_total counter\n"
                f"probe_cycles_success_total {cycles_success_total}\n"
            ).encode()

        self.send_response(200)
        self.send_header("Content-Type", "text/plain; version=0.0.4")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    threading.Thread(target=run_checks, daemon=True).start()
    http.server.ThreadingHTTPServer(
        ("0.0.0.0", 9115),
        MetricsHandler,
    ).serve_forever()