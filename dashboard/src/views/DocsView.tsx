import { useEffect, useMemo, useState } from 'react';
import { Search, BookOpen, Play, Filter, ChevronLeft, ChevronRight, FileText } from 'lucide-react';

interface Doc {
  name: string;
  path: string;
  product?: string;
  description?: string;
}

interface Flow {
  id: number;
  role_id: number;
  name: string;
  description: string;
  created_at: string;
}

interface Role {
  id: number;
  name: string;
}

interface Pagination {
  page: number;
  limit: number;
  total: number;
  total_pages: number;
}

interface DocsResponse {
  data?: Doc[];
  pagination?: Pagination;
}

const DOCS_LIMIT = 12;

function DocsSkeleton() {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {Array.from({ length: 6 }).map((_, idx) => (
        <div key={idx} className="card p-4 animate-pulse">
          <div className="h-5 w-24 bg-gray-200 rounded mb-3" />
          <div className="h-5 w-2/3 bg-gray-200 rounded mb-2" />
          <div className="h-4 w-full bg-gray-100 rounded mb-1" />
          <div className="h-4 w-4/5 bg-gray-100 rounded" />
        </div>
      ))}
    </div>
  );
}

export default function DocsView() {
  const [viewMode, setViewMode] = useState<'docs' | 'flows'>('docs');

  const [docs, setDocs] = useState<Doc[]>([]);
  const [docsLoading, setDocsLoading] = useState(true);
  const [docsPagination, setDocsPagination] = useState<Pagination>({
    page: 1,
    limit: DOCS_LIMIT,
    total: 0,
    total_pages: 1,
  });

  const [searchInput, setSearchInput] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [productFilter, setProductFilter] = useState('all');
  const [sortBy, setSortBy] = useState<'name' | 'name_desc' | 'product_asc' | 'product_desc'>('name');
  const [page, setPage] = useState(1);

  const [flows, setFlows] = useState<Flow[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [flowsLoading, setFlowsLoading] = useState(true);

  const fetchDocs = async () => {
    try {
      setDocsLoading(true);
      const params = new URLSearchParams();
      params.set('limit', String(DOCS_LIMIT));
      params.set('page', String(page));
      if (searchQuery) params.set('search', searchQuery);
      if (productFilter !== 'all') params.set('product', productFilter);
      if (sortBy) params.set('sort_by', sortBy);

      const res = await fetch(`/api/docs?${params.toString()}`);
      const payload: DocsResponse | Doc[] = await res.json();

      if (Array.isArray(payload)) {
        setDocs(payload);
        setDocsPagination({ page, limit: DOCS_LIMIT, total: payload.length, total_pages: 1 });
      } else {
        const data = Array.isArray(payload.data) ? payload.data : [];
        const pg = payload.pagination;
        setDocs(data);
        setDocsPagination({
          page: pg?.page ?? page,
          limit: pg?.limit ?? DOCS_LIMIT,
          total: pg?.total ?? data.length,
          total_pages: pg?.total_pages ?? 1,
        });
      }
    } catch (error) {
      console.error('Failed to fetch docs:', error);
      setDocs([]);
      setDocsPagination({ page: 1, limit: DOCS_LIMIT, total: 0, total_pages: 1 });
    } finally {
      setDocsLoading(false);
    }
  };

  const fetchFlows = async () => {
    try {
      setFlowsLoading(true);
      const [flowsRes, rolesRes] = await Promise.all([fetch('/api/flows'), fetch('/api/roles')]);
      const flowsData = await flowsRes.json();
      const rolesData = await rolesRes.json();
      setFlows(Array.isArray(flowsData) ? flowsData : []);
      setRoles(Array.isArray(rolesData) ? rolesData : []);
    } catch (error) {
      console.error('Failed to fetch flows/roles:', error);
      setFlows([]);
      setRoles([]);
    } finally {
      setFlowsLoading(false);
    }
  };

  useEffect(() => {
    if (viewMode === 'docs') {
      fetchDocs();
    }
  }, [viewMode, searchQuery, productFilter, sortBy, page]);

  useEffect(() => {
    if (viewMode === 'flows') {
      fetchFlows();
    }
  }, [viewMode]);

  const roleById = useMemo(() => {
    const map = new Map<number, string>();
    roles.forEach((r) => map.set(r.id, r.name));
    return map;
  }, [roles]);

  const availableProducts = useMemo(() => {
    const unique = new Set<string>();
    docs.forEach((d) => {
      if (d.product) unique.add(d.product);
    });
    return Array.from(unique).sort((a, b) => a.localeCompare(b));
  }, [docs]);

  const filteredFlows = useMemo(() => {
    const q = searchQuery.toLowerCase();
    return flows.filter((f) => {
      if (!q) return true;
      return f.name?.toLowerCase().includes(q) || f.description?.toLowerCase().includes(q);
    });
  }, [flows, searchQuery]);

  const applySearch = () => {
    setPage(1);
    setSearchQuery(searchInput.trim());
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Documentation</h2>
          <p className="text-gray-600">
            {viewMode === 'docs' ? 'Server-driven docs search/filter/pagination' : 'User flows overview'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setViewMode('docs')}
            className={`px-4 py-2 rounded-lg flex items-center gap-2 ${
              viewMode === 'docs' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700'
            }`}
          >
            <BookOpen className="w-4 h-4" /> Docs
          </button>
          <button
            onClick={() => setViewMode('flows')}
            className={`px-4 py-2 rounded-lg flex items-center gap-2 ${
              viewMode === 'flows' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700'
            }`}
          >
            <Play className="w-4 h-4" /> Flows
          </button>
        </div>
      </div>

      <div className="card mb-6 p-4">
        <div className="grid grid-cols-1 md:grid-cols-5 gap-3">
          <div className="md:col-span-2 relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
            <input
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') applySearch();
              }}
              placeholder={viewMode === 'docs' ? 'Search docs...' : 'Search flows...'}
              className="w-full pl-10 pr-3 py-2 border rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
            />
          </div>

          {viewMode === 'docs' && (
            <select
              value={productFilter}
              onChange={(e) => {
                setProductFilter(e.target.value);
                setPage(1);
              }}
              className="px-3 py-2 border rounded-lg"
            >
              <option value="all">All Products</option>
              {availableProducts.map((product) => (
                <option key={product} value={product}>
                  {product}
                </option>
              ))}
            </select>
          )}

          {viewMode === 'docs' && (
            <select
              value={sortBy}
              onChange={(e) => setSortBy(e.target.value as 'name' | 'name_desc' | 'product_asc' | 'product_desc')}
              className="px-3 py-2 border rounded-lg"
            >
              <option value="name">Name A-Z</option>
              <option value="name_desc">Name Z-A</option>
              <option value="product_asc">Product A-Z</option>
              <option value="product_desc">Product Z-A</option>
            </select>
          )}

          <button onClick={applySearch} className="btn btn-primary flex items-center justify-center gap-2">
            <Filter className="w-4 h-4" /> Search
          </button>
        </div>
      </div>

      {viewMode === 'docs' && (
        <>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
            <div className="card p-4">
              <div className="text-sm text-gray-500">Current Page Rows</div>
              <div className="text-2xl font-bold">{docs.length}</div>
            </div>
            <div className="card p-4">
              <div className="text-sm text-gray-500">Total Docs</div>
              <div className="text-2xl font-bold">{docsPagination.total}</div>
            </div>
            <div className="card p-4">
              <div className="text-sm text-gray-500">Page</div>
              <div className="text-2xl font-bold">
                {docsPagination.page} / {Math.max(1, docsPagination.total_pages)}
              </div>
            </div>
          </div>

          {docsLoading ? (
            <DocsSkeleton />
          ) : docs.length === 0 ? (
            <div className="card p-10 text-center text-gray-500">
              <FileText className="w-10 h-10 mx-auto mb-3 text-gray-300" />
              No documentation found for current query.
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {docs.map((doc) => (
                <div key={`${doc.path}-${doc.name}`} className="card p-4">
                  <div className="text-xs text-gray-500 mb-2">{doc.product || 'Unknown product'}</div>
                  <div className="font-medium text-gray-900 break-words">{doc.name || 'Untitled document'}</div>
                  <div className="text-sm text-gray-600 mt-2 break-words">{doc.path}</div>
                </div>
              ))}
            </div>
          )}

          <div className="mt-4 border-t bg-gray-50 px-4 py-3 flex items-center justify-between rounded-lg">
            <div className="text-sm text-gray-600">
              Total: {docsPagination.total} | Page {docsPagination.page} / {Math.max(1, docsPagination.total_pages)}
            </div>
            <div className="flex gap-2">
              <button
                onClick={() => setPage((prev) => Math.max(1, prev - 1))}
                disabled={docsPagination.page <= 1 || docsLoading}
                className="px-3 py-1.5 rounded border bg-white text-gray-700 disabled:opacity-50 flex items-center gap-1"
              >
                <ChevronLeft className="w-4 h-4" /> Prev
              </button>
              <button
                onClick={() => setPage((prev) => Math.min(docsPagination.total_pages || 1, prev + 1))}
                disabled={docsPagination.page >= docsPagination.total_pages || docsLoading}
                className="px-3 py-1.5 rounded border bg-white text-gray-700 disabled:opacity-50 flex items-center gap-1"
              >
                Next <ChevronRight className="w-4 h-4" />
              </button>
            </div>
          </div>
        </>
      )}

      {viewMode === 'flows' && (
        <div className="card overflow-hidden">
          <div className="card-header">
            <h3 className="font-semibold">Flows ({filteredFlows.length})</h3>
          </div>
          {flowsLoading ? (
            <div className="p-4"><DocsSkeleton /></div>
          ) : (
            <div className="divide-y divide-gray-200">
              {filteredFlows.length === 0 ? (
                <div className="p-8 text-center text-gray-500">No flows found</div>
              ) : (
                filteredFlows.map((flow) => (
                  <div key={flow.id} className="p-4 hover:bg-gray-50">
                    <div className="font-medium text-gray-900">{flow.name || 'Untitled flow'}</div>
                    <div className="text-sm text-gray-600">{flow.description || 'No description'}</div>
                    <div className="text-xs text-gray-500 mt-1">Role: {roleById.get(flow.role_id) || `Role ${flow.role_id}`}</div>
                  </div>
                ))
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
