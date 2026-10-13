# contrib

`release-workflow.yml` is the GitHub Actions workflow that builds the amd64/arm64
release binaries and attaches them to a tag.

GitHub refuses to let a fine-grained token create files under `.github/workflows/`
unless it carries the **Workflows: Read and write** permission. To enable it:

* **option A (manual):** in the web UI open *Add file → Create new file*, name it
  `.github/workflows/release.yml` and paste this file's content.
* **option B (token):** grant the token *Workflows: Read and write*, then:

  ```bash
  mkdir -p .github/workflows && cp contrib/release-workflow.yml .github/workflows/release.yml
  git add .github/workflows/release.yml && git commit -m "ci: release workflow" && git push
  ```

The installer works without it: `--bin-source auto` compiles from source on the server
(or downloads the release asset once a release exists).
