import { A, useParams } from '@solidjs/router'
import DataGrid from '../components/DataGrid'

export default function TableView() {
  const params = useParams()
  return (
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <h1 class="text-xl font-semibold">Table: {params.table}</h1>
      </div>
      <nav class="flex gap-2 text-xs">
        <A
          class="hover:underline"
          href={`/databases/${encodeURIComponent(params.dbId || '')}/tables/${encodeURIComponent(params.table || '')}`}
          end
        >
          Data
        </A>
        <A
          class="hover:underline"
          href={`/databases/${encodeURIComponent(params.dbId || '')}/tables/${encodeURIComponent(params.table || '')}/schema`}
        >
          Schema
        </A>
        <A
          class="hover:underline"
          href={`/databases/${encodeURIComponent(params.dbId || '')}/tables/${encodeURIComponent(params.table || '')}/audit`}
        >
          Audit
        </A>
        <A
          class="hover:underline"
          href={`/databases/${encodeURIComponent(params.dbId || '')}/tables/${encodeURIComponent(params.table || '')}/permissions`}
        >
          Permissions
        </A>
        <A
          class="hover:underline"
          href={`/databases/${encodeURIComponent(params.dbId || '')}/tables/${encodeURIComponent(params.table || '')}/import-export`}
        >
          Import/Export
        </A>
      </nav>
      <DataGrid dbId={params.dbId || ''} table={params.table || ''} />
    </div>
  )
}
