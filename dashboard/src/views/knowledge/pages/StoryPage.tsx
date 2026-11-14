import { useMemo } from 'react';
import { StoryDetail } from '../api';
import { CaseGridSection } from './CaseGrid';
import EntityShell from './EntityShell';
import { CaseRows, CoverageRow, CoverageSection, LinkedFeatures, SectionHead } from './kit';
import { PageCtx, crumbTo } from './data';
import UntestedPanel from './Untested';

export default function StoryPage({ ctx, e }: { ctx: PageCtx; e: StoryDetail }) {
  const { pid } = ctx;
  const epic = e.epic?.[0];
  const all = useMemo(() => e.test_cases ?? [], [e.test_cases]);
  const live = all.filter((c) => !c.removed_at);
  const done = live.filter((c) => c.status === 'automated');
  const todo = live.filter((c) => c.status !== 'automated');
  const gone = all.filter((c) => c.removed_at);
  const features = e.features ?? [];

  return (
    <EntityShell
      ctx={ctx}
      entity={e}
      title={e.text || e.code}
      crumbs={[...crumbTo(pid, 'epic', e.epic_code, epic?.name), { label: e.code, title: e.text }]}
      coverage={e.coverage ?? null}
      related={<LinkedFeatures pid={pid} refs={features} unmapped={e.unmapped_reason} />}
      details={[
        ['Epic', epic ? `${epic.code} ${epic.name}` : e.epic_code],
        ['Unmapped', e.unmapped_reason],
      ]}
      draftTab="stories"
    >
      {live.length === 0 ? (
        <UntestedPanel
          pid={pid}
          e={e}
          index={null}
          what="This user story has no test cases yet."
          todo={(catalog, sync) => (
            <>
              Add test cases under <span className="font-mono text-[0.8125rem]">{e.code}</span> in {catalog}, then {sync}.
            </>
          )}
        />
      ) : (
        <>
          <CaseGridSection pid={pid} cases={live} storyText={{ [e.code]: e.text }} empty="No test cases for this story yet." />

          <section aria-labelledby="kb-todo" className="space-y-3">
            <SectionHead id="kb-todo" title="Not automated yet" count={todo.length} />
            {todo.length ? (
              <div className="rounded-lg border border-gray-200 bg-white">
                <CaseRows pid={pid} cases={todo} />
              </div>
            ) : (
              <p className="text-sm text-gray-500">Every test case of this story is automated.</p>
            )}
          </section>

          {done.length > 0 && (
            <section aria-labelledby="kb-done" className="space-y-3">
              <SectionHead id="kb-done" title="Automated" count={done.length} />
              <div className="rounded-lg border border-gray-200 bg-white">
                <CaseRows pid={pid} cases={done} />
              </div>
            </section>
          )}
        </>
      )}

      {gone.length > 0 && (
        <section aria-labelledby="kb-gone" className="space-y-3">
          <SectionHead id="kb-gone" title="Removed from the repo" count={gone.length} />
          <div className="rounded-lg border border-gray-200 bg-white">
            <CaseRows pid={pid} cases={gone} />
          </div>
        </section>
      )}

      <CoverageSection
        id="kb-features"
        title="Linked features"
        items={features}
        noun="features"
        empty={e.unmapped_reason ? `Not linked to a feature (unmapped: ${e.unmapped_reason}).` : 'Not linked to a feature.'}
        render={(f) => <CoverageRow pid={pid} type="feature" code={f.code} name={f.name} origin={f.origin} removed={!!f.removed_at} coverage={f.coverage} />}
      />
    </EntityShell>
  );
}
