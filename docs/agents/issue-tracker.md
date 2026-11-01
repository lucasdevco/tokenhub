# Issue tracker: GitHub

Issues and specs live in `lucasdevco/tokenhub` on GitHub. Use the `gh` CLI with an explicit `--repo lucasdevco/tokenhub` target.

- Publish a spec with `gh issue create --repo lucasdevco/tokenhub --title "..." --body-file <file> --label ready-for-agent`.
- Read an issue with `gh issue view <number> --repo lucasdevco/tokenhub --json number,title,body,labels,comments`.
- List issues with `gh issue list --repo lucasdevco/tokenhub --state open --json number,title,labels`.
- Edit labels with `gh issue edit <number> --repo lucasdevco/tokenhub --add-label "..."` or `--remove-label "..."`.
- Use `--body-file` for multiline bodies and comments. Inspect existing issues before publishing to avoid duplicates.
- Before the first publication in a session, verify the destination with `gh repo view lucasdevco/tokenhub --json nameWithOwner`.
- Publishing to the issue tracker means creating a GitHub issue. Fetching a ticket means reading that GitHub issue.

## Pull requests as a triage surface

PRs as a request surface: no.
