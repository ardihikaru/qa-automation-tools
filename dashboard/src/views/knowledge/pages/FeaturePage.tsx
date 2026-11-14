import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { FeatureDetail, TestCase, entityHref } from '../api';
import { ReleaseChip } from '../ui';
import { CaseGridSection } from './CaseGrid';
import EntityShell from './EntityShell';
import { CaseRows, CoverageRow, CoverageSection } from './kit';
import { PageCtx, TreeIndex, crumbTo } from './data';
import UntestedPanel from './Untested';

export default function FeaturePage({ ctx, e, index }: { ctx: PageCtx; e: FeatureDetail; index: TreeIndex | null }) {
  const { pid } = ctx;
  const stories = useMemo(() => e.stories ?? [], [e.stories]);
  // Each test case belongs to one story, so this has no duplicates; dedupe anyway.
  const cases = useMemo(() => {
    const seen = new Map<string, TestCase>();
    for (const s of stories) for (const c of s.test_cases ?? []) if (!c.removed_at) seen.set(c.code, c);
    return [...seen.values()];
  }, [stories]);
  const storyText = useMemo(() => Object.fromEntries(stories.map((s) => [s.code, s.text])), [stories]);
  const mod = e.module?.[0];
  const sub = e.submodule?.[0];
  const partCode = index?.modules[e.module_code]?.part ?? null;

  return (
    <EntityShell
      ctx={ctx}
      entity={e}
      title={e.name || e.code}
      crumbs={[
        ...crumbTo(pid, 'part', partCode, partCode ? index?.parts[partCode] : null),
        ...crumbTo(pid, 'module', e.module_code, mod?.name ?? index?.modules[e.module_code]?.name),
        ...crumbTo(pid, 'submodule', e.submodule_code, sub?.name ?? (e.submodule_code ? index?.submodules[e.submodule_code]?.name : null)),
        ...crumbTo(pid, 'feature', e.code, e.name),
      ]}
      coverage={e.coverage ?? null}
      meta={<ReleaseChip status={e.release_status} asOf={e.status_as_of} />}
      details={[
        ['Release', e.release_status],
        ['Status as of', e.status_as_of],
        ['Module', mod ? `${mod.code} ${mod.name}` : e.module_code],
        ['Sub-module', sub ? `${sub.code} ${sub.name}` : e.submodule_code],
        ['Repo entry', e.repo_entry && <span className="font-mono text-xs break-all">{e.repo_entry}</span>],
        ['Evidence', e.evidence],
        ['Notes', e.notes],
      ]}
    >
      {cases.length === 0 ? (
        <UntestedPanel
          pid={pid}
          e={e}
          index={index}
          what={
            stories.length === 0
              ? 'No user story links to this feature, so nothing tests it yet.'
              : `${stories.length === 1 ? 'One user story links' : `${stories.length} user stories link`} to this feature, but ${stories.length === 1 ? 'it has' : 'none has'} test cases yet.`
          }
          todo={(catalog, sync) =>
            stories.length === 0 ? (
              <>
                To cover it, add a user story that lists <span className="font-mono text-[0.8125rem]">{e.code}</span> to {catalog} in the automation repo, give it test cases, then {sync}.
              </>
            ) : (
              <>
                Add test cases to {stories.length === 1 ? 'that story' : 'those stories'} in {catalog}, then {sync}.
              </>
            )
          }
          extra={
            stories.length > 0 && (
              <ul className="pt-1 space-y-1">
                {stories.map((s) => (
                  <li key={s.code} className="text-sm min-w-0 flex gap-2">
                    <Link to={entityHref(pid, 'story', s.code)} className="font-mono text-xs text-primary-700 hover:underline shrink-0 pt-0.5">
                      {s.code}
                    </Link>
                    <span className="text-gray-600 line-clamp-2 break-words" title={s.text}>
                      {s.text}
                    </span>
                  </li>
                ))}
              </ul>
            )
          }
          facts={
            <p className="flex flex-wrap items-center gap-2 text-sm text-gray-600">
              Release status <ReleaseChip status={e.release_status} asOf={e.status_as_of} />
              {e.status_as_of && <span className="text-gray-500">as of {e.status_as_of}</span>}
            </p>
          }
        />
      ) : (
        <>
          <CaseGridSection pid={pid} cases={cases} storyText={storyText} empty="No test cases cover this feature yet." />
          <CoverageSection
            id="kb-stories"
            title="User stories"
            items={stories}
            noun="user stories"
            empty="No user story references this feature yet."
            render={(s) => {
              const live = (s.test_cases ?? []).filter((c) => !c.removed_at);
              return (
                <div>
                  <CoverageRow pid={pid} type="story" code={s.code} name={s.text} origin={s.origin} removed={!!s.removed_at} coverage={s.coverage} noted={!!s.annotation} wrap />
                  {live.length > 0 ? (
                    <div className="ml-3 md:ml-8 mb-2 border-l border-gray-200">
                      <CaseRows pid={pid} cases={live} />
                    </div>
                  ) : (
                    <p className="ml-3 md:ml-8 mb-2 pl-3 text-xs text-gray-500 border-l border-gray-200">No test cases yet.</p>
                  )}
                </div>
              );
            }}
          />
        </>
      )}
    </EntityShell>
  );
}
