import { useState, useEffect } from 'react';
import { 
  Search, 
  FileText, 
  Calendar, 
  Filter,
  BookOpen,
  Play,
  Download,
  ExternalLink,
  ChevronRight,
  Clock,
  Users,
  Grid,
  List
} from 'lucide-react';

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
  updated_at: string;
  steps?: Step[];
}

interface Step {
  id: number;
  flow_id: number;
  step_number: number;
  action: string;
  selector: string;
  screenshot?: string;
  description: string;
}

interface Role {
  id: number;
  name: string;
  description: string;
  created_at: string;
}

export default function DocsView() {
  const [docs, setDocs] = useState<Doc[]>([]);
  const [flows, setFlows] = useState<Flow[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [roleFilter, setRoleFilter] = useState('all');
  const [selectedDoc, setSelectedDoc] = useState<Doc | null>(null);
  const [selectedFlow, setSelectedFlow] = useState<Flow | null>(null);
  const [viewMode, setViewMode] = useState<'docs' | 'flows'>('docs');
  const [dateFilter, setDateFilter] = useState('all');
  const [sortBy, setSortBy] = useState<'name' | 'date'>('date');
  const [displayMode, setDisplayMode] = useState<'grid' | 'list'>('grid');

  useEffect(() => {
    fetchData();
  }, [viewMode]);

  const fetchData = async () => {
    try {
      setLoading(true);
      
      if (viewMode === 'docs') {
        const docsRes = await fetch('/api/docs');
        const docsData = await docsRes.json();
        setDocs(Array.isArray(docsData) ? docsData : []);
      } else {
        // Fetch flows
        const flowsRes = await fetch('/api/flows');
        const flowsData = await flowsRes.json();
        setFlows(Array.isArray(flowsData) ? flowsData : []);
        
        // Fetch roles for role names
        const rolesRes = await fetch('/api/roles');
        const rolesData = await rolesRes.json();
        setRoles(Array.isArray(rolesData) ? rolesData : []);
      }
    } catch (error) {
      console.error('Failed to fetch data:', error);
    } finally {
      setLoading(false);
    }
  };

  // Get unique roles from docs/products
  const availableRoles = [...new Set(docs.map(d => d.product).filter(Boolean))];

  // Filter docs
  const filteredDocs = docs.filter(doc => {
    const matchesSearch = !search || 
      doc.name?.toLowerCase().includes(search.toLowerCase()) ||
      doc.product?.toLowerCase().includes(search.toLowerCase());
    
    const matchesRole = roleFilter === 'all' || doc.product?.toLowerCase() === roleFilter.toLowerCase();
    
    return matchesSearch && matchesRole;
  });

  // Filter flows
  const filteredFlows = flows.filter(flow => {
    const matchesSearch = !search || 
      flow.name?.toLowerCase().includes(search.toLowerCase()) ||
      flow.description?.toLowerCase().includes(search.toLowerCase());
    
    const matchesRole = roleFilter === 'all' || 
      roles.find(r => r.id === flow.role_id)?.name?.toLowerCase() === roleFilter.toLowerCase();
    
    return matchesSearch && matchesRole;
  }).sort((a, b) => {
    if (sortBy === 'name') {
      return (a.name || '').localeCompare(b.name || '');
    }
    return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
  });

  // Get role name from flow
  const getRoleName = (roleId: number) => {
    const role = roles.find(r => r.id === roleId);
    return role?.name || `Role ${roleId}`;
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return 'N/A';
    try {
      return new Date(dateStr).toLocaleDateString();
    } catch {
      return dateStr;
    }
  };

  const handleViewDoc = (doc: Doc) => {
    setSelectedDoc(doc);
  };

  const handleViewFlow = (flow: Flow) => {
    setSelectedFlow(flow);
  };

  const handleBack = () => {
    setSelectedDoc(null);
    setSelectedFlow(null);
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Documentation</h2>
          <p className="text-gray-600">
            {viewMode === 'docs' ? 'Browse system documentation by role' : 'View user flow documentation'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setViewMode('docs')}
            className={`px-4 py-2 rounded-lg flex items-center gap-2 ${
              viewMode === 'docs' 
                ? 'bg-primary-600 text-white' 
                : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
            }`}
          >
            <BookOpen className="w-4 h-4" />
            Docs
          </button>
          <button
            onClick={() => setViewMode('flows')}
            className={`px-4 py-2 rounded-lg flex items-center gap-2 ${
              viewMode === 'flows' 
                ? 'bg-primary-600 text-white' 
                : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
            }`}
          >
            <Play className="w-4 h-4" />
            Flows
          </button>
        </div>
      </div>

      {/* Search and Filters */}
      <div className="flex flex-col md:flex-row gap-4 mb-6">
        <div className="flex-1 relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
          <input
            type="text"
            placeholder={viewMode === 'docs' ? "Search documentation..." : "Search flows..."}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
          />
        </div>
        
        {/* Role Filter */}
        <div className="flex items-center gap-2">
          <Filter className="w-5 h-5 text-gray-400" />
          <select
            value={roleFilter}
            onChange={(e) => setRoleFilter(e.target.value)}
            className="px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
          >
            <option value="all">All Roles</option>
            {viewMode === 'docs' ? (
              availableRoles.map(role => (
                <option key={role} value={role}>{role}</option>
              ))
            ) : (
              roles.map(role => (
                <option key={role.id} value={role.name.toLowerCase()}>{role.name}</option>
              ))
            )}
          </select>
        </div>

        {/* Sort (for flows) */}
        {viewMode === 'flows' && (
          <select
            value={sortBy}
            onChange={(e) => setSortBy(e.target.value as 'name' | 'date')}
            className="px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
          >
            <option value="date">Newest First</option>
            <option value="name">Sort by Name</option>
          </select>
        )}

        {/* Display Mode Toggle */}
        <div className="flex border border-gray-300 rounded-lg overflow-hidden">
          <button
            onClick={() => setDisplayMode('grid')}
            className={`p-2 ${displayMode === 'grid' ? 'bg-primary-100' : 'bg-white'}`}
          >
            <Grid className="w-5 h-5 text-gray-600" />
          </button>
          <button
            onClick={() => setDisplayMode('list')}
            className={`p-2 ${displayMode === 'list' ? 'bg-primary-100' : 'bg-white'}`}
          >
            <List className="w-5 h-5 text-gray-600" />
          </button>
        </div>
      </div>

      {/* Stats Overview */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="card p-4">
          <div className="text-sm text-gray-500">{viewMode === 'docs' ? 'Total Docs' : 'Total Flows'}</div>
          <div className="text-2xl font-bold">{viewMode === 'docs' ? docs.length : flows.length}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Products/Roles</div>
          <div className="text-2xl font-bold">{viewMode === 'docs' ? availableRoles.length : roles.length}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Filtered</div>
          <div className="text-2xl font-bold">{viewMode === 'docs' ? filteredDocs.length : filteredFlows.length}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">
            {viewMode === 'docs' ? 'With Screenshots' : 'With Steps'}
          </div>
          <div className="text-2xl font-bold">
            {viewMode === 'docs' 
              ? docs.filter(d => d.path?.includes('screenshot')).length 
              : flows.length}
          </div>
        </div>
      </div>

      {/* Content Grid */}
      {(viewMode === 'docs' ? filteredDocs : filteredFlows).length === 0 ? (
        <div className="text-center py-12 text-gray-500">
          <FileText className="w-12 h-12 mx-auto mb-4 text-gray-300" />
          <p>No {viewMode === 'docs' ? 'documentation' : 'flows'} found</p>
          <p className="text-sm mt-2">Try adjusting your search or filters</p>
        </div>
      ) : displayMode === 'grid' ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {(viewMode === 'docs' ? filteredDocs : filteredFlows).map((item, idx) => (
            <div 
              key={idx} 
              className="card hover:shadow-md transition-shadow cursor-pointer"
              onClick={() => viewMode === 'docs' 
                ? handleViewDoc(item as Doc) 
                : handleViewFlow(item as Flow)
              }
            >
              <div className="card-body">
                <div className="flex items-center justify-between mb-2">
                  <span className="badge badge-primary capitalize">
                    {viewMode === 'docs' 
                      ? (item as Doc).product || 'general' 
                      : getRoleName((item as Flow).role_id)
                    }
                  </span>
                  <ChevronRight className="w-4 h-4 text-gray-400" />
                </div>
                <h3 className="text-lg font-semibold mb-2 truncate">
                  {(item as any).name || (item as any).title}
                </h3>
                <p className="text-gray-600 text-sm line-clamp-2">
                  {(item as any).description || 'No description available'}
                </p>
                <div className="mt-3 flex items-center gap-2 text-xs text-gray-400">
                  <Calendar className="w-3 h-3" />
                  {formatDate((item as any).created_at)}
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="card">
          <div className="divide-y divide-gray-200">
            {(viewMode === 'docs' ? filteredDocs : filteredFlows).map((item, idx) => (
              <div 
                key={idx}
                className="p-4 hover:bg-gray-50 cursor-pointer flex items-center gap-4"
                onClick={() => viewMode === 'docs' 
                  ? handleViewDoc(item as Doc) 
                  : handleViewFlow(item as Flow)
                }
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-1">
                    <span className="badge badge-primary capitalize text-xs">
                      {viewMode === 'docs' 
                        ? (item as Doc).product || 'general' 
                        : getRoleName((item as Flow).role_id)
                      }
                    </span>
                  </div>
                  <div className="font-medium truncate">
                    {(item as any).name || (item as any).title}
                  </div>
                  <div className="text-sm text-gray-500 truncate">
                    {(item as any).description || 'No description available'}
                  </div>
                </div>
                <div className="text-right text-sm text-gray-400">
                  <div className="flex items-center gap-1">
                    <Calendar className="w-3 h-3" />
                    {formatDate((item as any).created_at)}
                  </div>
                </div>
                <ChevronRight className="w-5 h-5 text-gray-400" />
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Pagination Info */}
      {(viewMode === 'docs' ? filteredDocs : filteredFlows).length > 0 && (
        <div className="mt-4 text-sm text-gray-500 text-center">
          Showing {Math.min(20, (viewMode === 'docs' ? filteredDocs : filteredFlows).length)} of{' '}
          {(viewMode === 'docs' ? filteredDocs : filteredFlows).length} results
        </div>
      )}
    </div>
  );
}