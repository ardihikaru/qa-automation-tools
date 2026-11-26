import { ModuleDetail, devPeriodRollup, periodText } from '../api';
import { ReleaseChip } from '../ui';
import { CaseGridSection } from './CaseGrid';
import EntityShell from './EntityShell';
import { CoverageRow, CoverageSection } from './kit';
import { PageCtx, TreeIndex, crumbTo, useScopeCases } from './data';
import UntestedPanel from './Untested';

export default function ModulePage({ ctx, e, index }: { ctx: PageCtx; e: ModuleDetail; index: TreeIndex | null }) {
  const { pid } = ctx;
  const g = useScopeCases(pid, { module: e.code }, e.coverage);
  const part = e.part?.[0];
  const subs = e.submodules ?? [];
  const subName = Object.fromEntries(subs.map((s) => [s.code, s.name]));
  const features = e.features ?? [];
  const noSub = features.filter((f) => !f.submodule_code).length;
  return (
    <EntityShell
      ctx={ctx}
      entity={e}
      title={e.name || e.code}
      crumbs={[...crumbTo(pid, 'part', e.part_code, part?.name ?? index?.parts[e.part_code]), ...crumbTo(pid, 'module', e.code, e.name)]}
      coverage={e.coverage ?? null}
      details={[
        ['Part', part ? `${part.code} ${part.name}` : e.part_code],
        ['Sub-modules', <span className="tabular-nums">{subs.length}</span>],
        ['Features', <span className="tabular-nums">{features.length}</span>],
        ['Periode pelaksanaan', periodText(devPeriodRollup(features))],
        ['No sub-module', noSub > 0 && <span className="tabular-nums">{noSub}</span>],
      ]}
    >
      {e.coverage?.test_cases === 0 ? (
        <UntestedPanel
          pid={pid}
          e={e}
          index={index}
          what={`None of the ${features.length === 1 ? 'one feature' : `${features.length} features`} in this module has test cases yet.`}
          todo={(catalog, sync) => (
            <>
              Link user stories with test cases to its features in {catalog}, then {sync}.
            </>
          )}
        />
      ) : (
        <CaseGridSection pid={pid} cases={g.cases} loading={g.loading} error={g.error} onRetry={g.reload} empty="No test cases cover this module yet." />
      )}
      {subs.length > 0 && (
        <CoverageSection
          id="kb-subs"
          title="Sub-modules"
          items={subs}
          noun="sub-modules"
          empty="No sub-modules."
          render={(s) => <CoverageRow pid={pid} type="submodule" code={s.code} name={s.name} origin={s.origin} removed={!!s.removed_at} coverage={s.coverage} />}
        />
      )}
      <CoverageSection
        id="kb-features"
        title="Features"
        items={features}
        noun="features"
        empty="No features in this module."
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
            extra={
              <span className="flex items-center gap-3 min-w-0">
                <span className="hidden lg:block text-xs text-gray-500 truncate max-w-[14rem]" title={f.submodule_code ? subName[f.submodule_code] ?? f.submodule_code : undefined}>
                  {f.submodule_code ? subName[f.submodule_code] ?? f.submodule_code : 'No sub-module'}
                </span>
                <ReleaseChip status={f.release_status} asOf={f.status_as_of} />
              </span>
            }
          />
        )}
      />
    </EntityShell>
  );
}
