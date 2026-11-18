"use client";

import { api } from "@multica/core/api";
import { useEffect, useRef, useState } from "react";
import type { SearchIssueResult, SearchProjectResult } from "@multica/core/types";
import { isLocalSearchReady, searchIssues, searchProjects } from "@multica/core/search-index";

export interface SearchResults {
  /**
   * The trimmed query these rows answer. Rows outlive the query that fetched
   * them — they stay painted while the next request is in flight — so every
   * result set carries the question it is the answer to, and anything that can
   * act on a row checks that it still matches what the user typed.
   */
  query: string;
  issues: SearchIssueResult[];
  projects: SearchProjectResult[];
}

export const NO_RESULTS: SearchResults = { query: "", issues: [], projects: [] };

/**
 * Issue (and optionally project) search for the palette. An empty query
 * clears the results without a request.
 */
export function useServerSearch(query: string, options: { projects: boolean; documentsOnly?: boolean }) {
  const [results, setResults] = useState<SearchResults>(NO_RESULTS);
  const [isLoading, setIsLoading] = useState(false);
  const withProjects = options.projects;
  const documentsOnly = options.documentsOnly ?? false;
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const q = query.trim();

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    if (abortRef.current) abortRef.current.abort();

    if (!q) {
      setResults(NO_RESULTS);
      setIsLoading(false);
      return;
    }

    setIsLoading(true);
    // The debounce spares the server a request per keystroke; the local index
    // answers in milliseconds, so it searches on every keystroke.
    const delay = isLocalSearchReady() ? 0 : 300;
    debounceRef.current = setTimeout(async () => {
      const controller = new AbortController();
      abortRef.current = controller;
      try {
        const [issueRes, documentRes, projectRes] = await Promise.all([
          documentsOnly ? Promise.resolve({ issues: [] as SearchIssueResult[] }) : searchIssues({ q, kind: "task", limit: 20, include_closed: true, signal: controller.signal }),
          api.searchIssues({ q, kind: "doc", limit: 20, include_closed: true, signal: controller.signal }),
          withProjects && !documentsOnly
            ? searchProjects({ q, limit: 10, include_closed: true, include_archived: true, signal: controller.signal })
            : Promise.resolve({ projects: [] as SearchProjectResult[] }),
        ]);
        if (!controller.signal.aborted) {
          setResults({ query: q, issues: [...issueRes.issues, ...documentRes.issues.filter((issue) => issue.kind === "doc")], projects: projectRes.projects });
          setIsLoading(false);
        }
      } catch {
        if (!controller.signal.aborted) {
          // Drop the previous query's rows rather than leaving them on screen
          // permanently greyed out: the request that would have replaced them
          // is never coming. The list falls through to the empty state.
          setResults({ query: q, issues: [], projects: [] });
          setIsLoading(false);
        }
      }
    }, delay);
  }, [q, withProjects, documentsOnly]);

  useEffect(
    () => () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      if (abortRef.current) abortRef.current.abort();
    },
    [],
  );

  // Rows answering an earlier query are still painted while the next request
  // is in flight — that is what keeps the list from strobing on every
  // keystroke — but they must not be actionable: cmdk re-selects the first
  // valid item whenever the input changes, so a live stale row turns the next
  // Enter into a jump to a result the user has already typed past.
  const stale = results.query !== q;
  return { results, isLoading, stale };
}
