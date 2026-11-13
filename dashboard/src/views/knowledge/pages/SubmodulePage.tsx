import { SubmoduleDetail } from '../api';
import { ReleaseChip } from '../ui';
import { CaseGridSection } from './CaseGrid';
import EntityShell from './EntityShell';
import { CoverageRow, CoverageSection } from './kit';
import { PageCtx, TreeIndex, crumbTo, useScopeCases } from './data';

export default function SubmodulePage({ ctx, e, index }: { ctx: PageCtx; e: SubmoduleDetail; index: TreeIndex | null }) {
  const { pid } = ctx;
  const g = useScopeCases(pid, { submodule: e.code }, e.coverage);
  const mod = e.module?.[0];
  const partCode = index?.modules[e.module_code]?.part ?? null;
  const features = e.features ?? [];
  return (
    <EntityShell
      ctx={ctx}
      entity={e}
      title={e.name || e.code}
      crumbs={[
        ...crumbTo(pid, 'part', partCode, partCode ? index?.parts[partCode] : null),
        ...crumbTo(pid, 'module', e.module_code, mod?.name ?? index?.modules[e.module_code]?.name),
        ...crumbTo(pid, 'submodule', e.code, e.name),
      ]}
      coverage={e.coverage ?? null}
      details={[
        ['Module', mod ? `${mod.code} ${mod.name}` : e.module_code],
        ['Features', <span className="tabular-nums">{features.length}</span>],
      ]}
    >
      <CaseGridSection pid={pid} cases={g.cases} loading={g.loading} error={g.error} onRetry={g.reload} empty="No test cases cover this sub-module yet." />
      <CoverageSection
        id="kb-features"
        title="Features"
        items={features}
        empty="No features in this sub-module."
        render={(f) => (
          <CoverageRow
            pid={pid}
            type="feature"
            code={f.code}
            name={f.name}
            origin={f.origin}
            removed={!!f.removed_at}
            coverage={f.coverage}
            noted={!!f.annotation}
            extra={<ReleaseChip status={f.release_status} asOf={f.status_as_of} />}
          />
        )}
      />
    </EntityShell>
  );
}
