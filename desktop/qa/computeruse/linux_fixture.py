#!/usr/bin/env python3
"""Account-free GTK fixture; emits an independent local widget-state oracle."""
import argparse
import json
from pathlib import Path
import gi
gi.require_version('Gtk', '3.0')
from gi.repository import Gtk


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--oracle', type=Path, required=True)
    args = parser.parse_args()
    state = dict(text='', clicks=0)
    def save():
        temporary = args.oracle.with_suffix('.tmp')
        temporary.write_text(json.dumps(state, ensure_ascii=False)+'\n')
        temporary.replace(args.oracle)
    window = Gtk.Window(title='Reasonix Cua Linux QA')
    window.set_default_size(640, 380)
    window.connect('destroy', Gtk.main_quit)
    content = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=20)
    content.set_border_width(40)
    window.add(content)
    content.pack_start(Gtk.Label(label='Reasonix Cua Linux 本地验收'), False, False, 0)
    entry = Gtk.Entry()
    entry.get_accessible().set_name('验收文本')
    def changed(widget):
        state['text'] = widget.get_text()
        save()
    entry.connect('changed', changed)
    content.pack_start(entry, False, False, 0)
    button = Gtk.Button(label='完成本地验收')
    output = Gtk.Label(label='等待本地验收')
    def clicked(_):
        state['clicks'] += 1
        output.set_text('本地验收完成 '+str(state['clicks']))
        save()
    button.connect('clicked', clicked)
    content.pack_start(button, False, False, 0)
    content.pack_start(output, False, False, 0)
    window.show_all()
    save()
    Gtk.main()


if __name__ == '__main__':
    main()
