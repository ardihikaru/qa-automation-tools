import { PartDetail } from '../api';
import { CaseGridSection } from './CaseGrid';
import EntityShell from './EntityShell';
import { CoverageRow, CoverageSection } from './kit';
import { PageCtx, TreeIndex, crumbTo, useScopeCases } from './data';
import UntestedPanel from './Untested';

export default function PartPage({ ctx, e, index }: { ctx: PageCtx; e: PartDetail; index: TreeIndex | null }) {
  const { pid } = ctx;
  const g = useScopeCases(pid, { part: e.code }, e.coverage);
  const modules = e.modules ?? [];
  return (
    <EntityShell
      ctx={ctx}
      entity={e}
      title={e.name || e.code}
      crumbs={crumbTo(pid, 'part', e.code, e.name)}
      coverage={e.coverage ?? null}
      details={[['Modules', <span className="tabular-nums">{modules.length}</span>]]}
    >
      {e.coverage?.test_cases === 0 ? (
        <UntestedPanel
          pid={pid}
          e={e}
          index={index}
          what={`None of the ${modules.length === 1 ? 'one module' : `${modules.length} modules`} in this part has test cases yet.`}
          todo={(catalog, sync) => (
            <>
              Link user stories with test cases to its features in {catalog}, then {sync}.
            </>
          )}
        />
      ) : (
        <CaseGridSection pid={pid} cases={g.cases} loading={g.loading} error={g.error} onRetry={g.reload} empty="No test cases cover this part yet." />
      )}
      <CoverageSection
        id="kb-children"
        title="Modules"
        items={modules}
        noun="modules"
        empty="No modules in this part."
        render={(m) => <CoverageRow pid={pid} type="module" code={m.code} name={m.name} origin={m.origin} removed={!!m.removed_at} coverage={m.coverage} />}
      />
    </EntityShell>
  );
}
