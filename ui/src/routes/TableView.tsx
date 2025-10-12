import { A, useParams } from '@solidjs/router'
import DataGrid from '../components/DataGrid'

export default function TableView() {
  const params = useParams()
  const cid = () => params.clusterId || ''
  const db = () => params.dbId || ''
  const tbl = () => params.table || ''
  return (
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <h1 class="text-xl font-semibold">Table: {tbl()}</h1>
      </div>
      <nav class="flex gap-2 text-xs">
        <A
          class="hover:underline"
          href={`/c/${encodeURIComponent(cid())}/databases/${encodeURIComponent(db())}/tables/${encodeURIComponent(tbl())}`}
          end
        >
          Data
        </A>
        <A
          class="hover:underline"
          href={`/c/${encodeURIComponent(cid())}/databases/${encodeURIComponent(db())}/tables/${encodeURIComponent(tbl())}/schema`}
        >
          Schema
        </A>
        <A
          class="hover:underline"
          href={`/c/${encodeURIComponent(cid())}/databases/${encodeURIComponent(db())}/tables/${encodeURIComponent(tbl())}/audit`}
        >
          Audit
        </A>
        <A
          class="hover:underline"
          href={`/c/${encodeURIComponent(cid())}/databases/${encodeURIComponent(db())}/tables/${encodeURIComponent(tbl())}/permissions`}
        >
          Permissions
        </A>
        <A
          class="hover:underline"
          href={`/c/${encodeURIComponent(cid())}/databases/${encodeURIComponent(db())}/tables/${encodeURIComponent(tbl())}/import-export`}
        >
          Import/Export
        </A>
      </nav>
      <DataGrid clusterId={cid()} dbId={db()} table={tbl()} />
    </div>
  )
}
