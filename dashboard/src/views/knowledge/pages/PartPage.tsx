import { PartDetail } from '../api';
import { CaseGridSection } from './CaseGrid';
import EntityShell from './EntityShell';
import { CoverageRow, CoverageSection } from './kit';
import { PageCtx, crumbTo, useScopeCases } from './data';

export default function PartPage({ ctx, e }: { ctx: PageCtx; e: PartDetail }) {
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
      <CaseGridSection pid={pid} cases={g.cases} loading={g.loading} error={g.error} onRetry={g.reload} empty="No test cases cover this part yet." />
      <CoverageSection
        id="kb-children"
        title="Modules"
        items={modules}
        empty="No modules in this part."
        render={(m) => <CoverageRow pid={pid} type="module" code={m.code} name={m.name} origin={m.origin} removed={!!m.removed_at} coverage={m.coverage} />}
      />
    </EntityShell>
  );
}
