import { useMemo } from 'react';
import { EpicDetail } from '../api';
import { CaseGridSection } from './CaseGrid';
import EntityShell from './EntityShell';
import { CoverageRow, CoverageSection } from './kit';
import { PageCtx, crumbTo, useScopeCases } from './data';
import UntestedPanel from './Untested';

export default function EpicPage({ ctx, e }: { ctx: PageCtx; e: EpicDetail }) {
  const { pid } = ctx;
  const g = useScopeCases(pid, { epic: e.code }, e.coverage);
  const stories = useMemo(() => e.stories ?? [], [e.stories]);
  const storyText = useMemo(() => Object.fromEntries(stories.map((s) => [s.code, s.text])), [stories]);
  return (
    <EntityShell
      ctx={ctx}
      entity={e}
      title={e.title || e.code}
      crumbs={crumbTo(pid, 'epic', e.code, e.title)}
      coverage={e.coverage ?? null}
      details={[
        ['Service', e.service],
        ['Area', e.area],
        ['User stories', <span className="tabular-nums">{stories.length}</span>],
      ]}
    >
      {e.coverage?.test_cases === 0 ? (
        <UntestedPanel
          pid={pid}
          e={e}
          index={null}
          what={stories.length ? `None of the ${stories.length === 1 ? 'one user story' : `${stories.length} user stories`} in this epic has test cases yet.` : 'This epic has no user stories or test cases yet.'}
          todo={(catalog, sync) => (
            <>
              Add test cases to its stories in {catalog}, then {sync}.
            </>
          )}
        />
      ) : (
        <CaseGridSection pid={pid} cases={g.cases} loading={g.loading} error={g.error} onRetry={g.reload} storyText={storyText} empty="No test cases in this epic yet." />
      )}
      <CoverageSection
        id="kb-stories"
        title="User stories"
        items={stories}
        noun="user stories"
        empty="No user stories in this epic."
        render={(s) => <CoverageRow pid={pid} type="story" code={s.code} name={s.text} origin={s.origin} removed={!!s.removed_at} coverage={s.coverage} noted={!!s.annotation} wrap />}
      />
    </EntityShell>
  );
}
