// Run history: every stored AI conversation that is not archived (AI_CONVERSATIONS.md section 6).
import { useCallback, useState } from 'react';
import { useAuth } from '../../auth/AuthContext';
import ConversationList from '../conversations/ConversationList';
import { ConversationPage, canReadMessages, listConversations } from '../conversations/api';
import { Filters, TabProps, useHistoryList } from './api';
import { FilterSelect, FilterText } from './filters';

const load = (f: Filters, s: AbortSignal): Promise<ConversationPage> => listConversations({ ...f, archived: 0 }, s);

export default function ConversationsTab({ filters, setFilter, refreshKey }: TabProps) {
  const { user } = useAuth();
  const [changed, setChanged] = useState(0);
  const { data, error, loading, retry } = useHistoryList(load, { kind: filters.kind ?? '', product: filters.product ?? '', q: filters.q ?? '', page: filters.page ?? '', page_size: filters.page_size }, refreshKey + changed);
  const onChanged = useCallback(() => setChanged((n) => n + 1), []);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <FilterSelect label="Kind" value={filters.kind ?? ''} options={[['job', 'Build'], ['sync', 'Knowledge sync'], ['call', 'Single call']]} onChange={(v) => setFilter('kind', v)} />
        <FilterText label="Job, sync or case" value={filters.q ?? ''} placeholder="job-… or TC-AUTH-001" onChange={(v) => setFilter('q', v)} />
      </div>
      <p className="text-sm text-gray-600">
        Every exchange the QA tools had with the AI, one conversation per build, knowledge sync or single call. Archived ones are on the Archive page.
        {!canReadMessages(user) && ' Your role can see this list but not the messages.'}
      </p>
      <ConversationList
        data={data}
        error={error}
        loading={loading}
        retry={retry}
        page={Number(filters.page ?? 1)}
        pageSize={Number(filters.page_size)}
        onPage={(p) => setFilter('page', String(p))}
        onPageSize={(s) => setFilter('page_size', String(s))}
        onChanged={onChanged}
        canAct={canReadMessages(user)}
        archived={false}
      />
    </div>
  );
}
