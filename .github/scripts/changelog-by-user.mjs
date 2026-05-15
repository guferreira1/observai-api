#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { appendFileSync, writeFileSync } from "node:fs";

const semverTagPattern = /^v?\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?$/;
const imageTag = process.env.IMAGE_TAG || process.env.GITHUB_REF_NAME || "HEAD";
const outputPath = process.env.CHANGELOG_OUTPUT || "release-notes.md";
const repository = process.env.GITHUB_REPOSITORY || "";
const githubToken = process.env.GITHUB_TOKEN || "";
const githubApiUrl = process.env.GITHUB_API_URL || "https://api.github.com";

function git(args, options = {}) {
  try {
    return execFileSync("git", args, {
      encoding: "utf8",
      stdio: ["ignore", "pipe", options.allowError ? "ignore" : "pipe"]
    }).trim();
  } catch (error) {
    if (options.allowError) {
      return "";
    }
    throw error;
  }
}

function refExists(ref) {
  return git(["rev-parse", "--verify", "--quiet", `${ref}^{commit}`], { allowError: true }) !== "";
}

function resolveCurrentRef(tag) {
  return refExists(`refs/tags/${tag}`) ? tag : "HEAD";
}

function semverTagsMergedInto(ref) {
  const output = git(["tag", "--merged", ref, "--sort=-version:refname"], { allowError: true });
  if (!output) {
    return [];
  }
  return output.split("\n").filter((tag) => semverTagPattern.test(tag));
}

function findPreviousTag(ref, tag, currentSha) {
  for (const candidate of semverTagsMergedInto(ref)) {
    if (candidate === tag) {
      continue;
    }

    const candidateSha = git(["rev-list", "-n", "1", candidate], { allowError: true });
    if (candidateSha && candidateSha !== currentSha) {
      return candidate;
    }
  }

  return "";
}

function commitShasForRange(previousTag, currentRef) {
  const range = previousTag ? `${previousTag}..${currentRef}` : currentRef;
  const output = git(["log", "--reverse", "--format=%H", range], { allowError: true });
  if (!output) {
    return [];
  }
  return output.split("\n").filter(Boolean);
}

function localCommitDetails(sha) {
  return {
    sha,
    subject: git(["show", "-s", "--format=%s", sha]),
    authorName: git(["show", "-s", "--format=%an", sha], { allowError: true }) || "unknown",
    authorEmail: git(["show", "-s", "--format=%ae", sha], { allowError: true })
  };
}

async function githubRequest(path, accept = "application/vnd.github+json") {
  if (!repository || !githubToken) {
    return null;
  }

  const response = await fetch(`${githubApiUrl}/repos/${repository}${path}`, {
    headers: {
      Accept: accept,
      Authorization: `Bearer ${githubToken}`,
      "X-GitHub-Api-Version": "2022-11-28"
    }
  });

  if (!response.ok) {
    return null;
  }

  return response.json();
}

async function associatedPullRequest(sha) {
  const pulls = await githubRequest(
    `/commits/${sha}/pulls`,
    "application/vnd.github+json"
  );
  if (!Array.isArray(pulls) || pulls.length === 0) {
    return null;
  }

  return pulls
    .slice()
    .sort((left, right) => {
      const leftNumber = Number(left.number || 0);
      const rightNumber = Number(right.number || 0);
      return rightNumber - leftNumber;
    })[0];
}

async function githubCommit(sha) {
  return githubRequest(`/commits/${sha}`);
}

function contributorKey(login, fallbackName) {
  if (login) {
    return `@${login}`;
  }
  return fallbackName || "unknown";
}

function contributorHeading(key) {
  if (key.startsWith("@")) {
    const login = key.slice(1);
    return `### [${key}](https://github.com/${login})`;
  }
  return `### ${key}`;
}

function cleanText(value) {
  return String(value || "").replace(/\s+/g, " ").trim();
}

function shortSha(sha) {
  return sha.slice(0, 7);
}

function addEntry(groups, key, entry) {
  if (!groups.has(key)) {
    groups.set(key, []);
  }
  groups.get(key).push(entry);
}

async function buildEntries(commitShas) {
  const groups = new Map();
  const seenPulls = new Set();

  for (const sha of commitShas) {
    const local = localCommitDetails(sha);
    const pullRequest = await associatedPullRequest(sha);

    if (pullRequest) {
      const pullKey = `pull:${pullRequest.number}`;
      if (seenPulls.has(pullKey)) {
        continue;
      }
      seenPulls.add(pullKey);

      const userKey = contributorKey(pullRequest.user?.login, local.authorName);
      addEntry(groups, userKey, {
        type: "pull",
        number: pullRequest.number,
        title: cleanText(pullRequest.title),
        url: pullRequest.html_url,
        sha
      });
      continue;
    }

    const commit = await githubCommit(sha);
    const userKey = contributorKey(commit?.author?.login, local.authorName);
    addEntry(groups, userKey, {
      type: "commit",
      title: cleanText(local.subject),
      url: commit?.html_url,
      sha
    });
  }

  return groups;
}

function renderMarkdown({ currentRef, currentSha, previousTag, commitShas, groups }) {
  const lines = [
    `# Release notes for ${imageTag}`,
    "",
    `Generated at: ${new Date().toISOString()}`,
    "",
    `- Previous tag: ${previousTag || "none"}`,
    `- Current ref: ${currentRef}`,
    `- Current commit: ${currentSha}`,
    `- Commit count: ${commitShas.length}`,
    "",
    "## Changes by GitHub user",
    ""
  ];

  if (commitShas.length === 0) {
    lines.push("No commits found for this release range.", "");
    return `${lines.join("\n")}\n`;
  }

  const sortedGroups = [...groups.entries()].sort(([left], [right]) => left.localeCompare(right));
  for (const [user, entries] of sortedGroups) {
    lines.push(contributorHeading(user), "");

    for (const entry of entries) {
      const suffix = entry.url
        ? `([${shortSha(entry.sha)}](${entry.url}))`
        : `(${shortSha(entry.sha)})`;

      if (entry.type === "pull") {
        lines.push(`- #${entry.number} ${entry.title} ${suffix}`);
      } else {
        lines.push(`- ${entry.title} ${suffix}`);
      }
    }

    lines.push("");
  }

  return `${lines.join("\n")}\n`;
}

const currentRef = resolveCurrentRef(imageTag);
const currentSha = git(["rev-parse", currentRef]);
const previousTag = findPreviousTag(currentRef, imageTag, currentSha);
const commitShas = commitShasForRange(previousTag, currentRef);
const groups = await buildEntries(commitShas);
const markdown = renderMarkdown({ currentRef, currentSha, previousTag, commitShas, groups });

writeFileSync(outputPath, markdown);

if (process.env.GITHUB_STEP_SUMMARY) {
  appendFileSync(process.env.GITHUB_STEP_SUMMARY, markdown);
}

console.log(`Generated ${outputPath} with ${commitShas.length} commit(s).`);
