import { Link } from 'react-router-dom';
import { TestCaseDetail, entityHref } from '../api';
import { Scenario } from '../EntityPanel';
import EntityShell from './EntityShell';
import { CoverageRow, CoverageSection, Fields, LinkedFeatures, SectionHead, StatusLabel } from './kit';
import { PageCtx, crumbTo } from './data';

const POLARITY: Record<string, string> = { '+': 'Positive (+)', '-': 'Negative (−)' };

export default function TestCasePage({ ctx, e }: { ctx: PageCtx; e: TestCaseDetail }) {
  const { pid } = ctx;
  const epic = e.epic?.[0];
  const story = e.story?.[0];
  const features = e.features ?? [];

  return (
    <EntityShell
      ctx={ctx}
      entity={e}
      title={e.code}
      codeIsTitle
      crumbs={[
        ...crumbTo(pid, 'epic', e.epic_code, epic?.name),
        ...(e.story_code ? [{ label: e.story_code, title: story?.name ?? e.story_code, to: entityHref(pid, 'story', e.story_code) }] : []),
        { label: e.code },
      ]}
      meta={<StatusLabel status={e.status} />}
      related={<LinkedFeatures pid={pid} refs={features} />}
      details={[
        ['Epic', epic ? `${epic.code} ${epic.name}` : e.epic_code],
        ['Story', e.story_code],
      ]}
      draftTab="test-cases"
    >
      <section aria-labelledby="kb-scenario" className="space-y-3">
        <SectionHead id="kb-scenario" title="Scenario" />
        <div className="rounded-lg border border-gray-200 bg-white px-4 py-3">
          <Scenario text={e.scenario} />
        </div>
      </section>

      <section aria-labelledby="kb-attrs" className="space-y-3">
        <SectionHead id="kb-attrs" title="Automation" />
        <Fields
          rows={[
            ['Status', <StatusLabel status={e.status} />],
            ['Automated', e.automated ? 'Yes' : 'Not automated yet'],
            ['Reason', [e.reason_category, e.reason_detail].filter(Boolean).join(': ')],
            ['Polarity', POLARITY[e.polarity] ?? e.polarity],
            ['Level', e.level],
            ['Priority', e.priority],
            [
              'Spec paths',
              e.spec_paths?.length ? (
                <ul className="space-y-0.5">
                  {e.spec_paths.map((p) => (
                    <li key={p} className="font-mono text-xs text-gray-700 break-all">
                      {p}
                    </li>
                  ))}
                </ul>
              ) : (
                <span className="text-gray-500">No spec yet</span>
              ),
            ],
          ]}
        />
      </section>

      <section aria-labelledby="kb-story" className="space-y-3">
        <SectionHead id="kb-story" title="User story" />
        {e.story_code ? (
          <Link to={entityHref(pid, 'story', e.story_code)} className="kb-row block rounded-lg border border-gray-200 bg-white px-3 py-2.5">
            <span className={`font-mono text-xs ${story?.removed_at ? 'line-through text-gray-400' : 'text-gray-500'}`}>{e.story_code}</span>
            {story?.name && <span className="block text-sm text-gray-900 break-words mt-0.5">{story.name}</span>}
          </Link>
        ) : (
          <p className="text-sm text-gray-500">Not linked to a story.</p>
        )}
      </section>

      <CoverageSection
        id="kb-features"
        title="Features"
        items={features}
        noun="features"
        empty="Its story is not linked to a feature."
        render={(f) => <CoverageRow pid={pid} type="feature" code={f.code} name={f.name} origin={f.origin} removed={!!f.removed_at} coverage={f.coverage} />}
      />

      <section aria-labelledby="kb-epic" className="space-y-3">
        <SectionHead id="kb-epic" title="Epic" />
        <Link to={entityHref(pid, 'epic', e.epic_code)} className="kb-row block rounded-lg border border-gray-200 bg-white px-3 py-2.5">
          <span className="font-mono text-xs text-gray-500">{e.epic_code}</span>
          {epic?.name && <span className="block text-sm text-gray-900 break-words mt-0.5">{epic.name}</span>}
        </Link>
      </section>
    </EntityShell>
  );
}
