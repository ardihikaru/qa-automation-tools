// Route `/products/:id/knowledge/:type/:code`: loads one entity and renders its page.
import { useEffect, useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useAuth } from '../../../auth/AuthContext';
import { Product, api } from '../../products/api';
import {
  Annotation,
  ApiError,
  CODE_RE,
  EntityDetail,
  EpicDetail,
  FeatureDetail,
  ModuleDetail,
  PartDetail,
  StoryDetail,
  SubmoduleDetail,
  TYPE_LABEL,
  TestCaseDetail,
  canWriteKnowledge,
  errorText,
  getEntity,
  isAbort,
  knowledgeHref,
  typeFromPath,
} from '../api';
import { ErrorNote, Loading } from '../ui';
import { Crumbs } from './kit';
import { PageCtx, baseCrumbs, topBarCrumbs, useTreeIndex } from './data';
import { usePageHeader } from '../../../components/pageHeader';
import PartPage from './PartPage';
import ModulePage from './ModulePage';
import SubmodulePage from './SubmodulePage';
import FeaturePage from './FeaturePage';
import EpicPage from './EpicPage';
import StoryPage from './StoryPage';
import TestCasePage from './TestCasePage';

export default function KnowledgeEntityPage() {
  const params = useParams();
  const pid = Number(params.id);
  const type = typeFromPath(params.type);
  const code = params.code ?? '';
  const valid = Number.isInteger(pid) && pid > 0 && !!type && CODE_RE.test(code);
  const { user } = useAuth();
  const canWrite = canWriteKnowledge(user?.role ?? '');
  const index = useTreeIndex(pid);

  const [product, setProduct] = useState<Product | null>(null);
  const [entity, setEntity] = useState<EntityDetail | null>(null);
  const [error, setError] = useState<{ msg: string; status?: number } | null>(null);
  const [retry, setRetry] = useState(0);

  // Each page is a fresh route mount (PageTransition keys on the path); start at the top.
  // Block body: newer browsers return a Promise from scrollTo, which must not become the cleanup.
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [type, code]);

  useEffect(() => {
    if (!Number.isInteger(pid) || pid <= 0) return;
    const ac = new AbortController();
    api<Product>(`/api/products/${pid}`, { signal: ac.signal })
      .then(setProduct)
      .catch(() => undefined);
    return () => ac.abort();
  }, [pid]);

  useEffect(() => {
    if (!valid || !type) return;
    const ac = new AbortController();
    setEntity(null);
    setError(null);
    getEntity(pid, type, code, ac.signal)
      .then((e) => !ac.signal.aborted && setEntity(e))
      .catch((e) => {
        if (ac.signal.aborted || isAbort(e)) return;
        setError({ msg: errorText(e, 'Could not load'), status: e instanceof ApiError ? e.status : undefined });
      });
    return () => ac.abort();
  }, [pid, type, code, valid, retry]);

  const ctx: PageCtx = useMemo(
    () => ({
      pid,
      productName: product?.name ?? null,
      canWrite,
      onAnnotation: (a: Annotation | null) => setEntity((cur) => (cur ? ({ ...cur, annotation: a } as EntityDetail) : cur)),
    }),
    [pid, product?.name, canWrite],
  );

  const label = type ? TYPE_LABEL[type] : 'Entry';
  const loaded = valid && !error && !!entity && entity.type === type && entity.code === code;
  // Until the page renders, this shell owns the top bar; then EntityShell takes over.
  usePageHeader(loaded ? null : { title: code || label, crumbs: topBarCrumbs(baseCrumbs(ctx)) });
  const crumbsRow = (
    <div className="lg:hidden">
      <Crumbs items={[...baseCrumbs(ctx), { label: code || label }]} />
    </div>
  );

  if (!valid || error) {
    const notFound = !valid || error?.status === 404;
    return (
      <div className="space-y-4">
        {crumbsRow}
        <ErrorNote onRetry={notFound ? undefined : () => setRetry((n) => n + 1)}>
          {notFound ? `${label} ${code} was not found in this knowledge base.` : error?.msg}
        </ErrorNote>
        {notFound && Number.isInteger(pid) && pid > 0 && (
          <Link to={knowledgeHref(pid)} className="text-sm text-primary-600 hover:underline">
            Back to the knowledge base
          </Link>
        )}
      </div>
    );
  }
  if (!entity || entity.type !== type || entity.code !== code) {
    return (
      <div className="space-y-4">
        {crumbsRow}
        <Loading />
      </div>
    );
  }

  switch (entity.type) {
    case 'part':
      return <PartPage ctx={ctx} e={entity as PartDetail} />;
    case 'module':
      return <ModulePage ctx={ctx} e={entity as ModuleDetail} index={index} />;
    case 'submodule':
      return <SubmodulePage ctx={ctx} e={entity as SubmoduleDetail} index={index} />;
    case 'feature':
      return <FeaturePage ctx={ctx} e={entity as FeatureDetail} index={index} />;
    case 'epic':
      return <EpicPage ctx={ctx} e={entity as EpicDetail} />;
    case 'story':
      return <StoryPage ctx={ctx} e={entity as StoryDetail} />;
    case 'test_case':
      return <TestCasePage ctx={ctx} e={entity as TestCaseDetail} />;
  }
}
