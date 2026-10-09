#!/usr/bin/env python3
"""Check the independently configured model through the real Go application.

Run via scripts/check-model.sh. Creates temporary data, sends the explicitly
labelled seed illustration to the configured provider, and never changes the
teaching database. This test can invoke the model up to seven times (one image
reading check and three report types, each with at most one structural repair).
No mock provider is started.
"""
import argparse
import base64
import json
import os
from pathlib import Path
import secrets
import subprocess
import struct
import tempfile
import time
import urllib.error
import urllib.request
import zlib

from e2e import API, ROOT, free_port


def check_vision(stream=True):
    """The expected digits exist only in the generated image, not the prompt."""
    font = [
        '11111100011000110001100011000111111', '00100011000010000100001000010001110',
        '11111000010000111111100001000011111', '11111000010000101111000010000111111',
        '10001100011000111111000010000100001', '11111100001000011111000010000111111',
        '11111100001000011111100011000111111', '11111000010001000100010000100001000',
        '11111100011000111111100011000111111', '11111100011000111111000010000111111',
    ]
    code = ''.join(secrets.choice('0123456789') for _ in range(5))
    width, height, scale = 580, 180, 18
    rows = []
    for y in range(height):
        row = bytearray([255] * width)
        for i, digit in enumerate(code):
            for gy in range(7):
                if 25 + gy * scale <= y < 25 + (gy + 1) * scale:
                    for gx in range(5):
                        if font[int(digit)][gy * 5 + gx] == '1':
                            start = 25 + i * 110 + gx * scale
                            row[start:start + scale] = bytes([0]) * scale
        rows.append(b'\0' + bytes(row))

    def chunk(tag, data):
        return struct.pack('!I', len(data)) + tag + data + struct.pack('!I', zlib.crc32(tag + data) & 0xffffffff)

    image = b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('!IIBBBBB', width, height, 8, 0, 0, 0, 0))
    image += chunk(b'IDAT', zlib.compress(b''.join(rows))) + chunk(b'IEND', b'')
    base = os.environ['ZHIWEI_AI_BASE_URL'].rstrip('/')
    endpoint = base if base.endswith('/chat/completions') else base + '/chat/completions'
    key = os.environ.get('ZHIWEI_AI_API_KEY', '')
    payload = {'model': os.environ['ZHIWEI_AI_MODEL'], 'stream': stream,
               'response_format': {'type': 'json_object'}, 'messages': [{
                   'role': 'user', 'content': [
                       {'type': 'text', 'text': '只读取所附图片中央的五位数字，返回JSON对象，唯一字段digits，值为五位数字字符串；如果你无法看见或读取图片，digits为null。不要猜测。'},
                       {'type': 'image_url', 'image_url': {'url': 'data:image/png;base64,' + base64.b64encode(image).decode()}},
                   ]}]}
    request = urllib.request.Request(endpoint, data=json.dumps(payload).encode(), headers={
        'Authorization': 'Bearer ' + key, 'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(request, timeout=int(os.environ.get('ZHIWEI_AI_TIMEOUT_SECONDS', '180'))) as response:
            if 'text/event-stream' in response.headers.get('Content-Type', ''):
                pieces, count = [], 0
                for line in response:
                    count += len(line)
                    if count > 1 << 20:
                        return {'vision_probe_error': 'response_too_large', 'matched': False}
                    if not line.startswith(b'data:'):
                        continue
                    data = line[5:].strip()
                    if data == b'[DONE]':
                        break
                    choices = json.loads(data).get('choices', [])
                    if choices:
                        pieces.append(choices[0].get('delta', {}).get('content') or '')
                content = ''.join(pieces)
            else:
                raw = response.read((1 << 20) + 1)
                if len(raw) > 1 << 20:
                    return {'vision_probe_error': 'response_too_large', 'matched': False}
                answer = json.loads(raw)
                content = answer['choices'][0]['message'].get('content', '')
            try:
                digits = json.loads(content).get('digits')
            except (ValueError, AttributeError):
                digits = None
            excerpt = content.replace(key, '[REDACTED]') if key else content
            return {'vision_probe_http': response.status, 'stream_requested': stream,
                    'expected_digits': code, 'returned_digits': digits,
                    'matched': digits == code, 'answer_excerpt': excerpt[:200]}
    except urllib.error.HTTPError as error:
        return {'vision_probe_http': error.code, 'matched': False}
    except (OSError, ValueError, KeyError, IndexError, TypeError) as error:
        return {'vision_probe_error': type(error).__name__, 'matched': False}


def wait_job(client, job, timeout):
    deadline = time.monotonic() + timeout + 20
    started = time.monotonic()
    last_stage = ''
    while time.monotonic() < deadline:
        current = client.request('GET', '/jobs/' + job['id'])
        stage = current.get('stage', '')
        if stage != last_stage:
            print(json.dumps({'job_kind': job['kind'], 'status': current['status'],
                              'stage': stage}, ensure_ascii=False), flush=True)
            last_stage = stage
        if current['status'] in ('succeeded', 'failed', 'cancelled'):
            current['elapsed_seconds'] = round(time.monotonic() - started, 1)
            return current
        time.sleep(1)
    raise TimeoutError('Local job did not reach a terminal state within the configured timeout.')


def run(binary):
    if not os.environ.get('ZHIWEI_AI_BASE_URL') or not os.environ.get('ZHIWEI_AI_MODEL'):
        raise RuntimeError('Model address and name must be configured in .env.')
    timeout = int(os.environ.get('ZHIWEI_AI_TIMEOUT_SECONDS', '180'))
    vision = check_vision()
    print(json.dumps(vision, ensure_ascii=False), flush=True)
    if not vision['matched']:
        print('Image reading did not pass. Report-format compatibility alone does not verify vision.', flush=True)
        return 1
    password = secrets.token_hex(18)
    with tempfile.TemporaryDirectory(prefix='zhiwei-live-model-') as directory:
        origin = 'http://127.0.0.1:' + str(free_port())
        env = dict(os.environ, ZHIWEI_ADDR=origin.removeprefix('http://'),
                   ZHIWEI_DATA_DIR=str(Path(directory) / 'data'),
                   ZHIWEI_WEB_DIR=str(ROOT / 'dist'), ZHIWEI_COOKIE_SECURE='false',
                   ZHIWEI_ADMIN_USERNAME='model-check-admin', ZHIWEI_ADMIN_PASSWORD=password)
        subprocess.run([str(binary), 'init-admin'], env=env, check=True, stdout=subprocess.DEVNULL)
        subprocess.run([str(binary), 'seed-demo'], env=env, check=True, stdout=subprocess.DEVNULL)
        with (Path(directory) / 'server.log').open('w+') as log:
            process = subprocess.Popen([str(binary), 'serve'], env=env, stdout=log, stderr=log)
            try:
                client = API(origin)
                for _ in range(100):
                    try:
                        client.request('GET', '/health')
                        break
                    except OSError:
                        if process.poll() is not None:
                            raise RuntimeError('Temporary application failed to start.')
                        time.sleep(.1)
                else:
                    raise RuntimeError('Temporary application did not become ready.')
                client.login('model-check-admin', password)
                atlas = next(item for item in client.request('GET', '/atlas')['items']
                             if item.get('demo') and item.get('file_id'))
                sample = client.request('POST', '/samples', {
                    'file_id': atlas['file_id'], 'title': '模型联调：合成示意图，非真实金相结论',
                    **{field: atlas.get(field) or '未知' for field in ('material', 'process', 'magnification', 'etchant')},
                })
                session = client.request('POST', '/learning-sessions', {'sample_id': sample['id']})
                results = []
                for kind in ('assessment', 'understanding', 'coaching'):
                    job = client.request('POST', '/jobs', {
                        'kind': kind, 'sample_id': sample['id'], 'learning_session_id': session['id'],
                        'idempotency_key': secrets.token_hex(16),
                    })
                    job = wait_job(client, job, timeout)
                    result = {'kind': kind, 'status': job['status'],
                              'elapsed_seconds': job['elapsed_seconds'], 'error': job.get('error')}
                    if job['status'] != 'succeeded':
                        results.append(result)
                        print(json.dumps({'results': results, 'scope': 'real model, temporary seeded illustration'}, ensure_ascii=False), flush=True)
                        return 1
                    report = client.request('GET', '/reports/' + job['result_id'])
                    result.update({'model': report['model'], 'demo': report.get('demo'),
                                   'citation_count': len(report.get('sources', [])),
                                   'limitations': report['data'].get('limitations', []),
                                   'summary': report['data'].get('summary', '')})
                    if kind == 'assessment':
                        result.update({'total_score': report['data'].get('total_score'),
                                       'coverage': report['data'].get('coverage')})
                    if kind == 'understanding':
                        result['dimensions'] = [{key: dim[key] for key in ('name', 'basis')}
                                                for dim in report['data']['dimensions']]
                    results.append(result)
                    if kind != 'coaching':
                        client.request('POST', '/reports/' + report['id'] + '/revisions', {
                            'data': report['data'], 'reason': '临时模型联调采用原稿，仅验证保存链路。',
                            'base_version': report['revision_no'],
                        })
                client.request('PATCH', '/learning-sessions/' + session['id'],
                               {'step': 'practice', 'completed': True})
                count = client.request('GET', '/personal-atlas')['total']
                print(json.dumps({'results': results, 'personal_atlas_count': count,
                                  'scope': 'real model and application pipeline; synthetic illustration only; no accuracy claim'},
                                 ensure_ascii=False, indent=2), flush=True)
                return 0
            finally:
                if process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=5)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, default=ROOT / 'bin/zhiwei')
    parser.add_argument('--vision-only', action='store_true', help='Only test reading random digits from an image.')
    args = parser.parse_args()
    if not args.binary.exists():
        parser.error('Build the Go server with make backend-build first.')
    try:
        if args.vision_only:
            result = check_vision()
            print(json.dumps(result, ensure_ascii=False), flush=True)
            raise SystemExit(0 if result['matched'] else 1)
        raise SystemExit(run(args.binary.resolve()))
    except (OSError, RuntimeError, TimeoutError, AssertionError, ValueError, StopIteration) as error:
        # Never print raw request contents, environment variables or credentials.
        print(json.dumps({'check_failed': type(error).__name__}, ensure_ascii=False), flush=True)
        raise SystemExit(1)
