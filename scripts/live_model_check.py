#!/usr/bin/env python3
"""Check the independently configured model through the real Go application.

Run via scripts/check-model.sh --image /path/to/authorized-photo.png. Creates
temporary data, uses the explicitly supplied photo, and never changes the
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
        with urllib.request.urlopen(request, timeout=int(os.environ.get('ZHIWEI_AI_TIMEOUT_SECONDS', '300'))) as response:
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


def call_diagnostics(path, job_id):
    """Expose measured times and schema codes only, never request/reply text."""
    allowed = ('msg', 'attempt', 'duration_ms', 'first_content_ms', 'finish_reason',
               'error_code', 'input_tokens', 'output_tokens', 'total_tokens',
               'valid', 'path', 'reason')
    result = []
    for line in path.read_text().splitlines():
        try:
            item = json.loads(line)
        except ValueError:
            continue
        if item.get('job_id') == job_id and item.get('msg') in ('ai.call.finished', 'ai.validation'):
            result.append({key: item[key] for key in allowed if key in item})
    return result


def run(binary, image_path, kind='all', reference_paths=()):
    if not os.environ.get('ZHIWEI_AI_BASE_URL') or not os.environ.get('ZHIWEI_AI_MODEL'):
        raise RuntimeError('Model address and name must be configured in .env.')
    timeout = int(os.environ.get('ZHIWEI_AI_TIMEOUT_SECONDS', '300'))
    mime = {'.jpg': 'image/jpeg', '.jpeg': 'image/jpeg', '.png': 'image/png', '.webp': 'image/webp'}.get(image_path.suffix.lower())
    if not mime or not image_path.is_file() or not 0 < image_path.stat().st_size <= 10 << 20:
        raise ValueError('Supply an authorized JPG, PNG or WebP photo up to 10 MiB.')
    image_bytes = image_path.read_bytes()
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
                   ZHIWEI_ADMIN_USERNAME='model-check-admin', ZHIWEI_ADMIN_PASSWORD=password,
                   ZHIWEI_LOG_FORMAT='json', ZHIWEI_LOG_LEVEL='debug')
        subprocess.run([str(binary), 'init-admin'], env=env, check=True, stdout=subprocess.DEVNULL)
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
                rubric = client.request('POST', '/admin/rubrics', {
                    'title': '临时模型诊断规则', 'description': '仅验证结构与保存链路，不作为正式教学依据。',
                    'criteria': [{'id': key, 'label': name, 'max_score': 25, 'rule_text': '仅在可见证据充分时给出学习建议分，不能判断时保留空值。'}
                                 for key, name in [('scratches', '划痕'), ('artifacts', '假象'), ('etching', '侵蚀程度'), ('clarity', '组织清晰与正确度')]],
                })
                client.request('POST', '/admin/rubrics/' + rubric['id'] + '/publish', {})
                image = client.upload(image_path.name, image_bytes, mime)
                sample = client.request('POST', '/samples', {
                    'file_id': image['id'], 'title': '临时模型联调照片',
                    **{field: '未知' for field in ('material', 'process', 'magnification', 'etchant')},
                })
                references = []
                for index, reference_path in enumerate(reference_paths):
                    reference_mime = {'.jpg': 'image/jpeg', '.jpeg': 'image/jpeg', '.png': 'image/png', '.webp': 'image/webp'}.get(reference_path.suffix.lower())
                    if not reference_mime or not reference_path.is_file() or not 0 < reference_path.stat().st_size <= 10 << 20:
                        raise ValueError('Reference photo must be JPG, PNG or WebP up to 10 MiB.')
                    reference_file = client.upload(reference_path.name, reference_path.read_bytes(), reference_mime)
                    entry = client.request('POST', '/atlas', {'title': f'联调参考图片 {index+1}', 'file_id': reference_file['id'], 'material': '未知', 'phase': '未作专业判定', 'process': '未知', 'magnification': '未知', 'etchant': '未知', 'source': '已上传照片，仅在隔离数据中验证参考请求链路，非组织标准', 'observation': '仅核对可见形貌；不提供组织真值'})
                    client.request('POST', '/atlas/' + entry['id'] + '/publish', {})
                    references.append(entry)
                session = client.request('POST', '/learning-sessions', {'sample_id': sample['id']})
                results = []
                kinds = ('assessment', 'understanding', 'coaching') if kind == 'all' else (kind,)
                for kind in kinds:
                    job = client.request('POST', '/jobs', {
                        'kind': kind, 'sample_id': sample['id'], 'learning_session_id': session['id'],
                        'idempotency_key': secrets.token_hex(16),
                        'input': {'question': '结合联调参考图片对照形貌，明确当前图与参考图的区别及判断局限。'} if references else {},
                    })
                    job = wait_job(client, job, timeout * 2)
                    result = {'kind': kind, 'status': job['status'],
                              'elapsed_seconds': job['elapsed_seconds'], 'error': job.get('error'),
                              'calls': call_diagnostics(Path(log.name), job['id'])}
                    if job['status'] != 'succeeded':
                        results.append(result)
                        print(json.dumps({'results': results, 'scope': 'real model, supplied photo, isolated temporary data'}, ensure_ascii=False), flush=True)
                        return 1
                    report = client.request('GET', '/reports/' + job['result_id'])
                    result.update({'model': report['model'], 'demo': report.get('demo'),
                                   'timings': report.get('timings'),
                                   'citation_count': len(report.get('sources', [])),
                                   'retrieved_source_kinds': [item.get('source_kind') for item in report.get('retrieved_sources', [])],
                                   'reference_image_count': max((item.get('details', {}).get('reference_count', 0) for item in job.get('progress_history', [])), default=0),
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
                if len(kinds) == 3:
                    client.request('PATCH', '/learning-sessions/' + session['id'],
                                   {'step': 'practice', 'completed': True})
                count = client.request('GET', '/personal-atlas')['total']
                print(json.dumps({'results': results, 'personal_atlas_count': count,
                                  'scope': 'real model and application pipeline; supplied photo with unknown background; no accuracy claim'},
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
    parser.add_argument('--image', type=Path, help='Authorized local photo for the full pipeline; background is recorded as unknown.')
    parser.add_argument('--reference-image', type=Path, action='append', default=[], help='Authorized reference photo for multimodal RAG integration; at most two, no accuracy claim.')
    parser.add_argument('--kind', choices=('all', 'assessment', 'understanding', 'coaching'), default='all', help='Limit diagnosis to one report type; all also verifies final archiving.')
    args = parser.parse_args()
    if len(args.reference_image) > 2:
        parser.error('At most two reference photos are supported.')
    if not args.binary.exists():
        parser.error('Build the Go server with make backend-build first.')
    if not args.vision_only and not args.image:
        parser.error('Supply --image /path/to/photo.png, or use --vision-only for the image-reading check.')
    try:
        if args.vision_only:
            result = check_vision()
            print(json.dumps(result, ensure_ascii=False), flush=True)
            raise SystemExit(0 if result['matched'] else 1)
        raise SystemExit(run(args.binary.resolve(), args.image.resolve(), args.kind, [path.resolve() for path in args.reference_image]))
    except (OSError, RuntimeError, TimeoutError, AssertionError, ValueError, StopIteration) as error:
        # Never print raw request contents, environment variables or credentials.
        print(json.dumps({'check_failed': type(error).__name__}, ensure_ascii=False), flush=True)
        raise SystemExit(1)
