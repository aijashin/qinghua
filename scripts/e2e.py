#!/usr/bin/env python3
"""Independent black-box acceptance test. Uses temporary data and a controlled
model HTTP server; never substitutes for live model/accuracy validation.

Run after `make build`: python3 scripts/e2e.py
"""
import argparse
import base64
import copy
import hashlib
import http.cookiejar
import http.server
import json
import os
from pathlib import Path
import re
import secrets
import socket
import struct
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
import urllib.parse
import zlib

ROOT = Path(__file__).resolve().parent.parent


def png(seed=0):
    def chunk(tag, body):
        return struct.pack('!I', len(body)) + tag + body + struct.pack('!I', zlib.crc32(tag + body) & 0xffffffff)
    size = 256
    rows = b''.join(b'\0' + bytes(((x * 7 + y * 11 + seed) % 256 for x in range(size))) for y in range(size))
    return b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('!IIBBBBB', size, size, 8, 0, 0, 0, 0)) + chunk(b'IDAT', zlib.compress(rows)) + chunk(b'IEND', b'')


def report(kind, citations):
    if kind == 'assessment':
        return {'summary': '受控测试输出：用于验证接口链路，不是实际金相结论。',
                'criteria': [{'id': key, 'label': name, 'score': 20 + i, 'max_score': 25, 'evidence': '受控测试图像证据'} for i, (key, name) in enumerate([
                    ('scratches', '划痕'), ('artifacts', '假象'), ('etching', '侵蚀程度'), ('clarity', '组织清晰与正确度')])],
                'defects': [{'name': '待核对纹理', 'severity': 'uncertain', 'evidence': '受控测试标记', 'suggestion': '补充另一视场核对。', 'bbox': {'x': .1, 'y': .1, 'width': .2, 'height': .3}}],
                'limitations': ['受控接口测试，不用于教学判读。'], 'citations': citations}
    if kind in ('understanding', 'identify'):
        return {'summary': '受控五维结构输出', 'candidates': [{'name': '待核对组织', 'evidence': '测试图像', 'uncertainty': '需要真实图像与教师核对'}],
                'dimensions': [{'name': name, 'content': f'{name}的受控测试内容', 'basis': basis} for name, basis in [('成分', 'known'), ('工艺', 'known'), ('组织', 'observed'), ('性能', 'inferred'), ('应用', 'inferred')]],
                'questions': ['还需要哪些证据？'], 'limitations': ['不用于实际教学结论。'], 'citations': citations}
    return {'summary': '受控复练建议', 'questions': ['你如何比较两个视场？'], 'suggestions': ['补充观察依据'], 'verification_plan': ['上传第二张图谱并比较纹理'], 'limitations': ['受控测试'], 'citations': citations}


class Fixture(http.server.BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    mode = 'ok'
    requests = []
    image_hashes = []

    def log_message(self, *_):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        Fixture.requests.append(body)
        text = json.dumps(body, ensure_ascii=False)
        match = re.search(r'task_kind:\s*(assessment|understanding|coaching|identify)', text)
        kind = match.group(1) if match else 'assessment'
        citations = []

        def walk(value):
            if isinstance(value, dict):
                if all(k in value for k in ('id', 'title', 'content')) and isinstance(value['id'], str):
                    citations.append(value['id'])
                if value.get('type') == 'image_url':
                    encoded = value['image_url']['url']
                    assert encoded.startswith('data:image/')
                    Fixture.image_hashes.append(hashlib.sha256(base64.b64decode(encoded.split(',', 1)[1])).hexdigest())
                for item in value.values():
                    walk(item)
            elif isinstance(value, list):
                for item in value:
                    walk(item)
            elif isinstance(value, str) and value.startswith('{'):
                try:
                    walk(json.loads(value))
                except (json.JSONDecodeError, TypeError):
                    pass

        walk(body)
        data = report(kind, list(dict.fromkeys(citations))[:1])
        mode = Fixture.mode
        if mode == 'invalid':
            data['citations'] = ['nonexistent-source']
        if mode == 'partial' and kind == 'assessment':
            data['criteria'][0]['score'] = None
            data['criteria'][0]['evidence'] = '当前视场无法观察此项，需要补充图片。'
        serialized = json.dumps(data, ensure_ascii=False)
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.send_header('Connection', 'close')
        self.end_headers()
        try:
            for i in range(0, len(serialized), 90):
                if mode == 'slow':
                    time.sleep(.25)
                event = {'choices': [{'index': 0, 'delta': {'content': serialized[i:i + 90]}, 'finish_reason': None}]}
                self.wfile.write(('data: ' + json.dumps(event) + '\n\n').encode())
                self.wfile.flush()
            if mode != 'truncated':
                self.wfile.write(b'data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n')
                self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass
        self.close_connection = True


class API:
    def __init__(self, origin):
        self.origin = origin
        self.csrf = ''
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def request(self, method, path, payload=None, expected=(200, 201, 202), raw=False, headers=None):
        body = payload if isinstance(payload, bytes) else None if payload is None else json.dumps(payload).encode()
        h = {'Origin': self.origin, 'X-CSRF-Token': self.csrf}
        if body is not None:
            h['Content-Type'] = 'application/json'
        h.update(headers or {})
        req = urllib.request.Request(self.origin + '/api/v1' + path, data=body, method=method, headers=h)
        try:
            response = self.opener.open(req, timeout=20)
        except urllib.error.HTTPError as error:
            response = error
        content = response.read()
        if response.status not in expected:
            raise AssertionError(f'{method} {path}: expected {expected}, got {response.status}: {content[:1200].decode(errors="replace")}')
        return content if raw else json.loads(content) if content else None

    def login(self, username, password):
        session = self.request('POST', '/auth/login', {'username': username, 'password': password})
        self.csrf = session['csrf_token']
        return session['user']

    def upload(self, name, content, mime):
        boundary = 'ZhiweiBoundary' + secrets.token_hex(8)
        body = f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{name}"\r\nContent-Type: {mime}\r\n\r\n'.encode() + content + f'\r\n--{boundary}--\r\n'.encode()
        return self.request('POST', '/files', body, headers={'Content-Type': f'multipart/form-data; boundary={boundary}'})

    def wait(self, job, status='succeeded'):
        until = time.monotonic() + 25
        while time.monotonic() < until:
            latest = self.request('GET', '/jobs/' + job['id'])
            if latest['status'] in ('succeeded', 'failed', 'cancelled'):
                assert latest['status'] == status, latest
                return latest
            time.sleep(.1)
        raise AssertionError('task did not finish: ' + job['id'])


def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def run(binary):
    fixture = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
    threading.Thread(target=fixture.serve_forever, daemon=True).start()
    password = 'E2E-' + secrets.token_hex(12)
    with tempfile.TemporaryDirectory(prefix='zhiwei-e2e-') as directory:
        data = Path(directory) / 'data'
        origin = f'http://127.0.0.1:{free_port()}'
        env = {**os.environ, 'ZHIWEI_DATA_DIR': str(data), 'ZHIWEI_ADDR': origin.removeprefix('http://'), 'ZHIWEI_WEB_DIR': str(ROOT / 'dist'),
               'ZHIWEI_ADMIN_USERNAME': 'admin', 'ZHIWEI_ADMIN_PASSWORD': password, 'ZHIWEI_AI_BASE_URL': f'http://127.0.0.1:{fixture.server_port}/v1',
               'ZHIWEI_AI_MODEL': 'controlled-test-model', 'ZHIWEI_AI_API_KEY': 'controlled-test-key', 'ZHIWEI_COOKIE_SECURE': 'false',
               'ZHIWEI_LOG_LEVEL': 'debug', 'ZHIWEI_LOG_FORMAT': 'text', 'ZHIWEI_LOG_AI_CONTENT': 'true'}
        subprocess.run([str(binary), 'init-admin'], env=env, check=True, stdout=subprocess.DEVNULL)
        subprocess.run([str(binary), 'seed-demo'], env=env, check=True, stdout=subprocess.DEVNULL)
        log = open(Path(directory) / 'server.log', 'w+')
        process = None

        def start():
            proc = subprocess.Popen([str(binary), 'serve'], env=env, stdout=log, stderr=log)
            for _ in range(100):
                try:
                    API(origin).request('GET', '/health')
                    return proc
                except (OSError, AssertionError):
                    if proc.poll() is not None:
                        break
                    time.sleep(.1)
            log.seek(0)
            raise AssertionError('server failed to start: ' + log.read()[-3000:])

        try:
            process = start()
            admin, student, peer, teacher, outsider = [API(origin) for _ in range(5)]
            admin.login('admin', password)
            assert not admin.request('GET', '/reports')['items'], 'demo seed must not fabricate AI reports'
            people = {}
            for name, role in [('student-a', 'student'), ('student-b', 'student'), ('teacher-a', 'teacher'), ('teacher-b', 'teacher')]:
                people[name] = admin.request('POST', '/admin/users', {'username': name, 'password': password, 'display_name': name, 'role': role})
            class_ = admin.request('POST', '/admin/classes', {'name': '受控验收班级'})
            for name in ('student-a', 'student-b', 'teacher-a'):
                admin.request('POST', f'/admin/classes/{class_["id"]}/members', {'user_id': people[name]['id']})
            for client, name in [(student, 'student-a'), (peer, 'student-b'), (teacher, 'teacher-a'), (outsider, 'teacher-b')]:
                client.login(name, password)
            student.request('GET', '/admin/users', expected=(403,))
            student.request('POST', '/samples', {}, expected=(403,), headers={'X-CSRF-Token': ''})
            uploaded = student.upload('controlled.png', png(), 'image/png')
            sample = student.request('POST', '/samples', {'file_id': uploaded['id'], 'title': '受控上传图谱', 'material': '未知', 'process': '未知', 'magnification': '未知', 'etchant': '未知', 'class_id': class_['id']})
            peer.request('GET', '/files/' + uploaded['id'], expected=(403, 404))
            outsider.request('GET', '/samples/' + sample['id'], expected=(403, 404))
            session = student.request('POST', '/learning-sessions', {'sample_id': sample['id']})
            request = {'kind': 'assessment', 'sample_id': sample['id'], 'learning_session_id': session['id'], 'idempotency_key': secrets.token_hex(16)}
            job = student.request('POST', '/jobs', request)
            duplicate = student.request('POST', '/jobs', request)
            assert duplicate['id'] == job['id'], 'double-submit created another task'
            student.request('POST', '/jobs', {**request, 'kind': 'understanding'}, expected=(409,))
            peer.request('GET', f'/jobs/{job["id"]}/events', expected=(403, 404))
            job = student.wait(job)
            assessment = student.request('GET', '/reports/' + job['result_id'])
            assert assessment['data']['total_score'] == 86, assessment
            assert assessment['data']['coverage'] == 1
            assert job.get('origin_request_id') and job.get('started_at') and job.get('finished_at'), 'job diagnostics were not persisted'
            assert job['timings'] == assessment['timings'], 'report and job timings disagree'
            assert job['timings']['total_ms'] >= 0 and job['timings']['queue_ms'] >= 0 and job['timings']['model_ms'] >= 0
            assert len(job['timings']['model_calls']) == 1, 'normal report must make one real provider call'
            assert [stage['code'] for stage in job['progress_history']] == ['queued', 'preparing', 'retrieving', 'model_waiting', 'model_receiving', 'validating', 'persisting', 'succeeded'], 'progress must reflect real processing boundaries'
            assert Fixture.image_hashes and hashlib.sha256(png()).hexdigest() in Fixture.image_hashes, 'uploaded image was not sent to the model'
            events = student.request('GET', f'/jobs/{job["id"]}/events', raw=True).decode()
            ids = [int(x) for x in re.findall(r'^id: ?(\d+)', events, re.M)]
            assert ids == sorted(set(ids)) and 'event: completed' in events, events[:800]
            assert 'event: progress' in events and 'model_receiving' in events, 'structured progress did not reach protected SSE'
            assert '受控测试图像证据' not in events and 'controlled-test-key' not in events, 'terminal-only AI content leaked into public progress'
            if len(ids) > 1:
                replay = student.request('GET', f'/jobs/{job["id"]}/events?after={ids[-2]}', raw=True).decode()
                assert all(int(x) > ids[-2] for x in re.findall(r'^id: ?(\d+)', replay, re.M)), replay
            changed = copy.deepcopy(assessment['data'])
            changed['criteria'][0]['score'] = 18
            changed['defects'][0]['bbox'] = {'x': .7, 'y': .6, 'width': .1, 'height': .1}
            revision = student.request('POST', f'/reports/{assessment["id"]}/revisions', {'data': changed, 'reason': '根据图像证据调整划痕得分', 'comment': '独立验收修正', 'base_version': assessment.get('revision_no', 0)})
            assert revision['data']['defects'][0]['bbox'] == assessment['data']['defects'][0]['bbox'], 'student revision replaced immutable observation coordinates'
            refreshed = student.request('GET', '/reports/' + assessment['id'])
            assert refreshed['data']['criteria'][0]['score'] == 20, 'student edit overwrote AI original'
            student.request('POST', f'/reports/{assessment["id"]}/revisions', {'data': changed, 'reason': '过期修改', 'base_version': 0}, expected=(409,))
            teacher.request('POST', f'/reports/{assessment["id"]}/viewed', {})
            teacher.request('POST', f'/reports/{assessment["id"]}/feedback', {'comment': '请对照另一视场说明依据。'})
            assert student.request('GET', '/reports/' + assessment['id'])['status'] != 'reviewed'
            teacher.request('POST', f'/reports/{assessment["id"]}/review', {'decision': 'confirmed', 'comment': '已核对学习记录，不是实验验证。'})
            understanding_job = student.request('POST', '/jobs', {**request, 'kind': 'understanding', 'idempotency_key': secrets.token_hex(16)})
            understanding_job = student.wait(understanding_job)
            understanding = student.request('GET', '/reports/' + understanding_job['result_id'])
            assert {d['name'] for d in understanding['data']['dimensions']} == {'成分', '工艺', '组织', '性能', '应用'}
            student.request('POST', f'/reports/{understanding["id"]}/revisions', {'data': understanding['data'], 'reason': '采用AI参考', 'comment': '采用AI参考', 'base_version': understanding.get('revision_no', 0), 'phase': '待核对组织'})
            student.request('PATCH', '/learning-sessions/' + session['id'], {'step': 'practice', 'completed': True})
            personal = student.request('GET', '/personal-atlas')['items']
            assert personal, 'completed learning did not create a personal atlas record'
            assert student.request('GET', '/personal-atlas?q=' + urllib.parse.quote('待核对组织'))['items'], 'personal atlas cannot be searched by the saved phase'
            task = teacher.request('POST', '/practice-tasks', {'title': '比较两张图谱', 'instruction': '说明划痕与纹理的证据差别', 'class_id': class_['id'], 'assignee_ids': [people['student-a']['id'], people['student-b']['id']]})
            teacher.request('POST', f'/practice-tasks/{task["id"]}/publish', {})
            student.request('POST', f'/practice-tasks/{task["id"]}/attempts', {'answer': '先在另一视场确认纹理是否穿过晶粒边界。'})
            student.request('POST', f'/practice-tasks/{task["id"]}/complete', {'reflection': '补充对照证据'})
            peer_task = peer.request('GET', f'/practice-tasks/{task["id"]}')
            assert not any(x.get('user_id') == people['student-a']['id'] for x in peer_task.get('assignments', [])), 'another student received private completion/reflection data'
            second_file = student.upload('controlled-next.png', png(3), 'image/png')
            second_sample = student.request('POST', '/samples', {'file_id': second_file['id'], 'title': '下一轮受控图谱', 'material': '未知', 'process': '未知', 'magnification': '未知', 'etchant': '未知', 'class_id': class_['id']})
            next_session = student.request('POST', '/learning-sessions', {'sample_id': second_sample['id'], 'parent_session_id': session['id'], 'task_id': task['id']})
            assert next_session.get('parent_session_id') == session['id']
            Fixture.mode = 'invalid'
            bad = student.request('POST', '/jobs', {'kind': 'assessment', 'sample_id': second_sample['id'], 'idempotency_key': secrets.token_hex(16)})
            failed = student.wait(bad, 'failed')
            assert not failed.get('result_id'), 'invalid model output produced a report'
            assert failed.get('error_code') and sum(item['code'] == 'repairing' for item in failed['progress_history']) == 1, 'schema failure must have one actual repair and a diagnostic error'
            Fixture.mode = 'truncated'
            truncated = student.request('POST', '/jobs', {'kind': 'assessment', 'sample_id': second_sample['id'], 'idempotency_key': secrets.token_hex(16)})
            student.wait(truncated, 'failed')
            Fixture.mode = 'slow'
            slow = student.request('POST', '/jobs', {'kind': 'assessment', 'sample_id': second_sample['id'], 'idempotency_key': secrets.token_hex(16)})
            time.sleep(.15)
            student.request('POST', f'/jobs/{slow["id"]}/cancel', {})
            cancelled = student.wait(slow, 'cancelled')
            assert cancelled['error_code'] == 'cancelled' and cancelled['progress']['code'] == 'cancelled'
            Fixture.mode = 'ok'
            docfile = admin.upload('验收资料.md', '# 贝氏体判读\n\n贝氏体与珠光体的区别需要结合工艺和可见形貌，本文仅供受控检索验收。'.encode(), 'text/markdown')
            imported = admin.request('POST', '/admin/documents', {'file_id': docfile['id'], 'title': '受控中文检索资料'})
            imported = admin.wait(imported)
            document_id = imported['result_id']
            admin.request('GET', f'/documents/{document_id}/chunks')
            admin.request('POST', f'/admin/documents/{document_id}/publish', {})
            hits = student.request('GET', '/knowledge/search?q=' + urllib.parse.quote('贝氏体'))['items']
            assert any(hit.get('document_id') == document_id for hit in hits), 'Chinese published document not found'
            updated = student.request('PATCH', '/samples/' + second_sample['id'], {'material': '贝氏体'})
            grounded = student.wait(student.request('POST', '/jobs', {'kind': 'assessment', 'sample_id': updated['id'], 'idempotency_key': secrets.token_hex(16)}))
            grounded_report = student.request('GET', '/reports/' + grounded['result_id'])
            assert grounded_report['sources'] and grounded_report['data']['citations'], 'the actual retrieved sources were not passed to the provider'
            source = grounded_report['data']['citations'][0]
            assert student.request('GET', '/knowledge/chunks/' + source)['content']
            replacement_file = admin.upload('新版本.md', '# 贝氏体新版\n\n贝氏体教学资料的新版本，仅供验证引用版本保留。'.encode(), 'text/markdown')
            replacement = admin.wait(admin.request('POST', '/admin/documents', {'file_id': replacement_file['id'], 'title': '受控中文检索资料新版', 'replaces_document_id': document_id}))
            admin.request('POST', f'/admin/documents/{replacement["result_id"]}/publish', {})
            new_hits = student.request('GET', '/knowledge/search?q=' + urllib.parse.quote('贝氏体'))['items']
            assert not any(hit.get('document_id') == document_id for hit in new_hits), 'archived document remains in public retrieval'
            assert student.request('GET', '/knowledge/chunks/' + source)['content'], 'historical cited version became unreadable'
            Fixture.mode = 'partial'
            partial = student.wait(student.request('POST', '/jobs', {'kind': 'assessment', 'sample_id': updated['id'], 'idempotency_key': secrets.token_hex(16)}))
            partial_report = student.request('GET', '/reports/' + partial['result_id'])
            assert partial_report['data']['total_score'] is None and partial_report['data']['coverage'] == .75, 'unobservable score became a comparable total'
            Fixture.mode = 'ok'
            insight = student.request('GET', '/insights')
            assert insight['sample_count'] == 2 and insight['report_count'] >= 2, insight
            process.terminate()
            process.wait(timeout=10)
            process = start()
            assert student.request('GET', '/reports/' + assessment['id'])['data']['total_score'] == 86
            assert student.request('GET', '/personal-atlas')['items']
            assert student.request('GET', '/jobs/' + job['id'])['timings'] == job['timings'], 'diagnostics did not survive restart'
            process.terminate()
            process.wait(timeout=10)
            backup = Path(directory) / 'backup.tar.gz'
            subprocess.run([str(binary), 'backup', str(backup)], env=env, check=True, stdout=subprocess.DEVNULL)
            env['ZHIWEI_DATA_DIR'] = str(Path(directory) / 'restored')
            subprocess.run([str(binary), 'restore', str(backup)], env=env, check=True, stdout=subprocess.DEVNULL)
            env['ZHIWEI_AI_BASE_URL'] = ''
            env['ZHIWEI_AI_MODEL'] = ''
            env['ZHIWEI_AI_API_KEY'] = ''
            process = start()
            assert not student.request('GET', '/auth/session')['ai_configured']
            student.request('POST', '/jobs', request, expected=(503,))
            assert student.request('GET', '/reports/' + assessment['id'])['data']['total_score'] == 86
            assert student.request('GET', '/files/' + uploaded['id'], raw=True) == png(), 'backup omitted or damaged original image'
            log.flush()
            logtext = Path(directory, 'server.log').read_text()
            for forbidden in ('controlled-test-key', password, 'data:image/png;base64,'):
                assert forbidden not in logtext, 'secret or image payload appeared in terminal diagnostics'
            for required in ('http.completed', 'ai.call.started', 'ai.call.finished', 'ai.content', 'job.stage', 'job.finished', 'call_id=', 'request_id=', '│ '):
                assert required in logtext, f'terminal diagnostics missing {required}'
            assert '受控测试图像证据' in logtext, 'explicit DEBUG content switch did not record the actual final answer'
            print('PASS: terminal diagnostics/redaction, structured progress/timings, immutable bbox, upload, identity/class isolation, CSRF, actual image request, idempotency, SSE replay, revisions, teacher feedback/review, five dimensions, atlas, practice privacy, next cycle, invalid/truncated/cancelled/partial AI, Chinese knowledge and immutable citations, statistics, restart, backup/restore and missing-model behavior.')
            print('Model: controlled HTTP fixture only; live model capability and metallographic accuracy are not asserted.')
        except Exception:
            log.flush()
            log.seek(0)
            print(log.read()[-2500:])
            raise
        finally:
            if process is not None and process.poll() is None:
                process.terminate()
                process.wait(timeout=10)
            log.close()
    fixture.shutdown()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, default=ROOT / 'bin/zhiwei')
    parser.add_argument('--fixture-port', type=int, help='Run only the controlled model HTTP fixture on localhost for browser QA.')
    args = parser.parse_args()
    if args.fixture_port is not None:
        fixture = http.server.ThreadingHTTPServer(('127.0.0.1', args.fixture_port), Fixture)
        print(f'Controlled model fixture only: http://127.0.0.1:{fixture.server_port}/v1', flush=True)
        try:
            fixture.serve_forever()
        except KeyboardInterrupt:
            fixture.server_close()
        raise SystemExit(0)
    if not args.binary.exists():
        parser.error('Run make build before this acceptance test.')
    run(args.binary.resolve())
