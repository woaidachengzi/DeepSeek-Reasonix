#!/usr/bin/env python3
"""Restore stable Wails topic titles in an isolated Preview profile.

The stable profile is opened read-only. Preview writes are opt-in (--apply),
preceded by a consistent SQLite backup and a JSON change journal. Rollback
uses compare-and-swap so later Preview edits are never silently overwritten.
"""

import argparse
import datetime as dt
import json
import pathlib
import shutil
import sqlite3
import sys


def read_db(path):
    return sqlite3.connect(f"file:{path}?mode=ro", uri=True)


def topic_records(stable):
    projects = json.loads((stable / "desktop-projects.json").read_text())
    groups = [("", projects.get("globalTopics", []))]
    groups += [(project["root"], project.get("topics", [])) for project in projects.get("projects", [])]
    topics = {}
    for root, ids in groups:
        db = stable / "desktop/topic-state-v1.sqlite" if not root else stable / "projects" / root.replace("/", "-") / "desktop/topic-state-v1.sqlite"
        if not db.is_file():
            continue
        with read_db(db) as con:
            for topic_id in ids:
                row = con.execute("SELECT title, created_at_ms FROM topics WHERE topic_id=?", (topic_id,)).fetchone()
                if row:
                    topics[topic_id] = {"root": root, "title": row[0], "createdAtMs": row[1]}
    return projects, topics


def valid_title(title):
    return isinstance(title, str) and 0 < len(title) <= 120 and title.strip() == title and not any(ord(c) < 32 or ord(c) == 127 for c in title)


def mapped_topic(stable, session_id, root, topics):
    manifest = stable / "desktop-sessions-v5/by-id" / session_id / "manifest.json"
    if not manifest.is_file():
        return None
    data = json.loads(manifest.read_text())
    source = data.get("source") or {}
    source_path = source.get("path", "") if isinstance(source, dict) else ""
    if source_path:
        sidecar = pathlib.Path(source_path + ".meta")
        if sidecar.is_file():
            try:
                topic_id = json.loads(sidecar.read_text()).get("topic_id", "")
                topic = topics.get(topic_id)
                if topic and (topic["root"] == root or not topic["root"] and root == str(stable / "global-workspace")):
                    return topic_id
            except (OSError, ValueError):
                return None
        return None
    created = data.get("createdAt")
    if not created:
        return None
    try:
        created_ms = int(dt.datetime.fromisoformat(created.replace("Z", "+00:00")).timestamp() * 1000)
    except ValueError:
        return None
    matches = [topic_id for topic_id, topic in topics.items()
               if (topic["root"] == root or not topic["root"] and root == str(stable / "global-workspace"))
               and abs(topic["createdAtMs"] - created_ms) <= 2000]
    return matches[0] if len(matches) == 1 else None


def proposed_folders(projects, stable, current):
    desired = [{"root": str(stable / "global-workspace"), "title": "Global"}]
    desired += [{"root": project["root"], "title": project.get("title", "")} for project in projects.get("projects", [])]
    seen = {item["root"] for item in desired}
    for item in current.get("projects", []):
        if isinstance(item, dict) and item.get("root") and item["root"] not in seen:
            desired.append({"root": item["root"], "title": item.get("title", "")})
            seen.add(item["root"])
    return {"projects": desired}


def backup_preview(preview, destination, project_path):
    destination.mkdir(parents=True, exist_ok=False)
    db_path = preview / "desktop/session-state-v1.sqlite"
    with read_db(db_path) as source, sqlite3.connect(destination / "session-state-v1.sqlite") as target:
        source.backup(target)
    if project_path.is_file():
        shutil.copy2(project_path, destination / "desktop-projects.json")


def sync_host_catalog(preview, backup_dir):
    """Keep the older host catalog in step so shadow audit has no title drift."""
    journal_path = backup_dir / "journal.json"
    journal = json.loads(journal_path.read_text())
    host_path = preview.parent / "workbench-sessions.json"
    if not host_path.is_file():
        return
    if not (backup_dir / "workbench-sessions.json").is_file():
        shutil.copy2(host_path, backup_dir / "workbench-sessions.json")
    catalog = json.loads(host_path.read_text())
    if not isinstance(catalog, list):
        raise ValueError("Preview host catalog must be a list")
    changes_by_id = {item["sessionId"]: item for item in journal["changes"]}
    host_changes = journal.get("hostCatalogChanges", [])
    already_changed = {item["sessionId"] for item in host_changes}
    with read_db(preview / "desktop/session-state-v1.sqlite") as con:
        identity_titles = dict(con.execute("SELECT id,title FROM sessions WHERE state='ready'"))
    for item in catalog:
        if not isinstance(item, dict):
            continue
        if item.get("sessionId") in already_changed:
            continue
        change = changes_by_id.get(item.get("sessionId"))
        if change and item.get("title", "") in ("", change["before"]):
            host_changes.append({"sessionId": change["sessionId"], "before": item.get("title"), "after": change["after"]})
            item["title"] = change["after"]
        elif not item.get("title") and identity_titles.get(item.get("sessionId")):
            title = identity_titles[item["sessionId"]]
            host_changes.append({"sessionId": item["sessionId"], "before": item.get("title"), "after": title})
            item["title"] = title
    journal["hostCatalogChanges"] = host_changes
    journal_path.write_text(json.dumps(journal, ensure_ascii=False, indent=2))
    temporary = host_path.with_name(".workbench-sessions.topic-migration.tmp")
    temporary.write_text(json.dumps(catalog, ensure_ascii=False, indent=2) + "\n")
    temporary.replace(host_path)
    print(f"Host catalog titles recorded in migration journal: {len(host_changes)}.")


def apply(stable, preview, write):
    projects, topics = topic_records(stable)
    db_path = preview / "desktop/session-state-v1.sqlite"
    with read_db(db_path) as con:
        rows = con.execute("SELECT id,title,title_source,title_revision,workspace_root,state FROM sessions").fetchall()
    changes = []
    matched = []
    for session_id, title, source, revision, root, state in rows:
        if state != "ready":
            continue
        topic_id = mapped_topic(stable, session_id, root, topics)
        if not topic_id:
            continue
        stable_title = topics[topic_id]["title"]
        matched.append((session_id, topic_id))
        if source == "fallback" and valid_title(stable_title) and title != stable_title:
            changes.append({"sessionId": session_id, "topicId": topic_id,
                            "before": title, "after": stable_title, "revision": revision})
    project_path = preview / "desktop-projects.json"
    current_projects = json.loads(project_path.read_text()) if project_path.is_file() else {}
    folders = proposed_folders(projects, stable, current_projects)
    print(json.dumps({"stableTopics": len(topics), "matchedReadySessions": len(matched),
                      "titleChanges": len(changes), "projectFolders": len(folders["projects"]),
                      "unmatchedReadySessions": sum(row[5] == "ready" for row in rows) - len(matched)}, indent=2))
    if not write:
        return
    stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    backup_dir = preview / "backups" / f"topic-migration-{stamp}"
    backup_preview(preview, backup_dir, project_path)
    journal = {"changes": changes, "projectFileExisted": project_path.is_file(), "foldersAfter": folders}
    (backup_dir / "journal.json").write_text(json.dumps(journal, ensure_ascii=False, indent=2))
    with sqlite3.connect(db_path, timeout=30) as con:
        con.execute("BEGIN IMMEDIATE")
        for change in changes:
            result = con.execute("UPDATE sessions SET title=?, title_source='legacy_unknown', title_revision=title_revision+1 "
                                 "WHERE id=? AND state='ready' AND title_source='fallback' AND title_revision=? AND title=?",
                                 (change["after"], change["sessionId"], change["revision"], change["before"]))
            if result.rowcount != 1:
                raise RuntimeError(f"session changed during migration: {change['sessionId']}")
        con.commit()
    temporary = project_path.with_name(".desktop-projects.topic-migration.tmp")
    temporary.write_text(json.dumps(folders, ensure_ascii=False, indent=2) + "\n")
    temporary.replace(project_path)
    sync_host_catalog(preview, backup_dir)
    print(f"Backup and change journal: {backup_dir}")


def rollback(preview, backup_dir):
    journal = json.loads((backup_dir / "journal.json").read_text())
    db_path = preview / "desktop/session-state-v1.sqlite"
    with sqlite3.connect(db_path, timeout=30) as con:
        con.execute("BEGIN IMMEDIATE")
        for change in journal["changes"]:
            result = con.execute("UPDATE sessions SET title=?, title_source='fallback', title_revision=title_revision+1 "
                                 "WHERE id=? AND title=? AND title_source='legacy_unknown' AND title_revision=?",
                                 (change["before"], change["sessionId"], change["after"], change["revision"] + 1))
            if result.rowcount != 1:
                raise RuntimeError(f"session changed since migration: {change['sessionId']}; rollback stopped")
        con.commit()
    project_path = preview / "desktop-projects.json"
    if project_path.is_file() and json.loads(project_path.read_text()) == journal["foldersAfter"]:
        if journal["projectFileExisted"]:
            shutil.copy2(backup_dir / "desktop-projects.json", project_path)
        else:
            project_path.unlink()
    host_path = preview.parent / "workbench-sessions.json"
    if host_path.is_file() and journal.get("hostCatalogChanges"):
        catalog = json.loads(host_path.read_text())
        before = {item["sessionId"]: item for item in journal["hostCatalogChanges"]}
        for item in catalog:
            change = before.get(item.get("sessionId")) if isinstance(item, dict) else None
            if change and item.get("title") == change["after"]:
                if change["before"] is None:
                    item.pop("title", None)
                else:
                    item["title"] = change["before"]
        temporary = host_path.with_name(".workbench-sessions.topic-rollback.tmp")
        temporary.write_text(json.dumps(catalog, ensure_ascii=False, indent=2) + "\n")
        temporary.replace(host_path)
    print("Rolled back matching titles and unchanged project folders.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--stable", type=pathlib.Path, default=pathlib.Path.home() / ".reasonix")
    parser.add_argument("--preview", type=pathlib.Path, default=pathlib.Path.home() / "Library/Application Support/io.reasonix.desktop.preview/reasonix-core")
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--rollback", type=pathlib.Path, help="backup directory produced by --apply")
    parser.add_argument("--sync-host", type=pathlib.Path, help="finish synchronizing the host catalog for an existing backup")
    args = parser.parse_args()
    try:
        if args.rollback:
            rollback(args.preview, args.rollback)
        elif args.sync_host:
            sync_host_catalog(args.preview, args.sync_host)
        else:
            apply(args.stable, args.preview, args.apply)
    except (OSError, ValueError, sqlite3.Error, RuntimeError) as error:
        print(f"migration failed: {error}", file=sys.stderr)
        sys.exit(1)
