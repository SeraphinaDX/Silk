#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""POSIX PTY checks: native page text, image-only graphics, links and forms."""
import fcntl
import os
from pathlib import Path
import pty
import re
import select
import signal
import struct
import subprocess
import tempfile
import termios
import time
ROOT = Path(__file__).resolve().parents[1]
DCS = re.compile(rb'\x1bP.*?\x1b\\', re.S)


def exercise(mode, with_image=True, exit_signal=False, missing_chrome=False):
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 30, 120, 960, 480))
    original = termios.tcgetattr(slave)
    with tempfile.TemporaryDirectory() as directory:
        fixture = Path(directory) / 'test.html'
        second = Path(directory) / 'second.html'
        second.write_text('<title>Second</title><h1>Second page native text</h1>')
        canvas = '<canvas id=c width=120 height=60></canvas>' if with_image else ''
        fixture.write_text('''<!doctype html><meta charset=utf-8><title>Ready</title>
<h1>Native heading CAFÉ</h1><p>Readable terminal characters.</p>
<p><button id=b>Click</button></p><p><input id=i placeholder="Name"></p>'''+canvas+'''
<p><a href="second.html">Next page</a></p><div id=long></div><script>
b.onclick=()=>b.textContent='Clicked';i.oninput=()=>document.title='Typed:'+i.value;
if(window.c){const ctx=c.getContext('2d');ctx.fillStyle='blue';ctx.fillRect(0,0,120,60)}
long.innerHTML=Array.from({length:80},(_,i)=>'<p>Scroll line '+i+'</p>').join('');
</script>''')
        args = [str(ROOT/'silk'), '-graphics='+mode, '-cell=8x16', str(fixture)]
        if missing_chrome:
            args.insert(1, '-chrome=/nonexistent/silk-test-chrome')
        elif os.getenv('SILK_CHROME'):
            args.insert(1, '-chrome='+os.environ['SILK_CHROME'])
        if os.getenv('SILK_TEST_NO_SANDBOX') == '1':
            args.insert(1, '-no-sandbox')
        proc = subprocess.Popen(args, stdin=slave, stdout=slave, stderr=slave, cwd=ROOT)
        captured = bytearray()
        all_output = bytearray()

        def read(timeout=.1):
            ready, _, _ = select.select([master], [], [], timeout)
            if ready:
                data=os.read(master,262144)
                captured.extend(data)
                all_output.extend(data)

        def wait_for(marker, timeout=10):
            deadline = time.monotonic()+timeout
            while marker not in captured:
                if time.monotonic() >= deadline:
                    raise AssertionError(f'{mode}: missing {marker!r}; tail={bytes(captured[-800:])!r}')
                read()

        def send(data):
            captured.clear()
            os.write(master,data)

        def click_text(text):
            # Find a native span's last cursor position in a full redraw. This
            # resolves the Go text layout, not Chromium's original CSS position.
            wait_for(text)
            plain=DCS.sub(b'',bytes(captured))
            pos=plain.rfind(text)
            cursors=list(re.finditer(rb'\x1b\[(\d+);1H',plain[:pos]))
            row=int(cursors[-1].group(1))
            send(f'\x1b[<0;3;{row}M\x1b[<0;3;{row}m'.encode())

        try:
            wait_for(b'\x1b[?1049h')
            if missing_chrome:
                wait_for(b'\x1b[?1049l')
                assert proc.wait(timeout=5)!=0
            else:
                wait_for('Native heading CAFÉ'.encode())
                wait_for(b'Readable terminal characters.')
                assert b'Readable terminal characters.' in DCS.sub(b'',bytes(captured))
                if mode=='auto' and with_image:
                    send(b'\x1b[?1;2;4c\x1b[6;16;8t')
                if with_image and mode in ('sixel','auto'):
                    wait_for(b'\x1bP0;1;0q"1;1;120;60')
                if with_image and mode=='halfblock':
                    wait_for('▀'.encode())
                click_text(b'[ Click ]')
                wait_for(b'[ Clicked ]')
                click_text(b'[Name: ]')
                send(b'\x1b[200~hello\x1b[201~')
                wait_for(b'[Name: hello]')
                send(b'\x1b')
                time.sleep(.1)
                read()
                click_text(b'Next page')
                wait_for(b'Second page native text')
                send(b'\x02')
                wait_for(b'Native heading')
                send(b'\x1b[<65;60;18M')
                wait_for(b'Scroll line')
                send(b'\x1b[F')
                wait_for(b'Scroll line 79')
                fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH',24,100,800,384))
                time.sleep(.6)
                send(b'\x0c\x1b[200~about:blank\x1b[201~\r')
                wait_for(b'about:blank')
                if exit_signal:
                    proc.send_signal(signal.SIGTERM)
                else:
                    os.write(master,b'\x11')
                wait_for(b'\x1b[?1049l')
                assert proc.wait(timeout=5)==0
                blocks=DCS.findall(bytes(all_output))
                if not with_image or mode=='none':
                    assert not blocks,'text-only output contains raster graphics'
                else:
                    for block in blocks:
                        dims=re.search(rb'"1;1;(\d+);(\d+)',block)
                        assert dims and int(dims[1])<=120 and int(dims[2])<=60,'whole page rasterized'
                assert b'Readable terminal characters.' in DCS.sub(b'',bytes(all_output))
            assert termios.tcgetattr(slave)==original,'terminal attributes not restored'
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait()
            os.close(master)
            os.close(slave)
    print(f'PASS {mode}, images={with_image}: native text, bounded images, controls, scrolling, cleanup')


if __name__=='__main__':
    exercise('sixel',with_image=False)
    exercise('sixel')
    exercise('none')
    exercise('halfblock',exit_signal=True)
    exercise('auto')
    exercise('sixel',missing_chrome=True)
