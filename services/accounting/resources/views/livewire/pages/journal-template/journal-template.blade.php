<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Transaction" subjudul="Template & Jurnal Berulang" :breadcrumb="['Transaction', 'Template & Jurnal Berulang']" />
    </x-slot>

    @if ($message)
        <div class="alert alert-success alert-dismissible">{{ $message }}</div>
    @endif
    @if ($error)
        <div class="alert alert-danger">{{ $error }}</div>
    @endif

    @if ($mode === 'template')
        <div class="card">
            <div class="card-header"><h5 class="card-title">{{ $editId ? 'Ubah' : 'Tambah' }} Template Jurnal</h5></div>
            <div class="card-body">
                <div class="row">
                    <div class="col-md-4 form-group">
                        <label>Nama template</label>
                        <input type="text" class="form-control" wire:model="tpl.name" maxlength="100">
                        @error('tpl.name') <span class="text-danger small">{{ $message }}</span> @enderror
                    </div>
                    <div class="col-md-3 form-group">
                        <label>Tipe transaksi</label>
                        <select class="form-control" wire:model="tpl.trans_code">
                            <option value="">- pilih -</option>
                            @foreach ($types as $t)
                                <option value="{{ $t->trans_code }}">{{ $t->trans_code }} - {{ $t->trans_name }}</option>
                            @endforeach
                        </select>
                        @error('tpl.trans_code') <span class="text-danger small">{{ $message }}</span> @enderror
                    </div>
                    <div class="col-md-5 form-group">
                        <label>Keterangan</label>
                        <input type="text" class="form-control" wire:model="tpl.description" maxlength="250">
                    </div>
                </div>
                <div class="form-inline mb-2">
                    <input type="text" class="form-control form-control-sm mr-2" placeholder="No. jurnal yang disalin" wire:model="copyFrom">
                    <button type="button" class="btn btn-sm btn-light" wire:click="copyFromJournal">Salin baris dari jurnal</button>
                    @error('copyFrom') <span class="text-danger small ml-2">{{ $message }}</span> @enderror
                </div>
                <table class="table table-sm">
                    <thead class="thead-light"><tr><th>Akun</th><th>Unit</th><th>Keterangan</th><th class="text-right">Debit</th><th class="text-right">Kredit</th><th></th></tr></thead>
                    <tbody>
                        @foreach ($tpl['lines'] as $i => $line)
                            <tr wire:key="tpl-line-{{ $i }}">
                                <td>
                                    <select class="form-control form-control-sm" wire:model="tpl.lines.{{ $i }}.acc_code">
                                        <option value="">- akun -</option>
                                        @foreach ($accounts as $a)
                                            <option value="{{ $a->acc_code }}">{{ $a->acc_code }} - {{ $a->acc_name }}{{ $a->unit_scope ? ' ['.$a->unit_scope.']' : '' }}</option>
                                        @endforeach
                                    </select>
                                </td>
                                <td>
                                    <select class="form-control form-control-sm" wire:model="tpl.lines.{{ $i }}.div_code">
                                        <option value="">- unit -</option>
                                        @foreach ($divisions as $d)
                                            <option value="{{ $d->div_code }}">{{ $d->div_code }} - {{ $d->div_name }}</option>
                                        @endforeach
                                    </select>
                                </td>
                                <td><input type="text" class="form-control form-control-sm" wire:model="tpl.lines.{{ $i }}.description" maxlength="250"></td>
                                <td><input type="number" step="0.01" min="0" class="form-control form-control-sm text-right" wire:model="tpl.lines.{{ $i }}.debit"></td>
                                <td><input type="number" step="0.01" min="0" class="form-control form-control-sm text-right" wire:model="tpl.lines.{{ $i }}.credit"></td>
                                <td><button type="button" class="btn btn-sm btn-link text-danger" wire:click="removeLine({{ $i }})" title="Hapus baris">&times;</button></td>
                            </tr>
                            @error('tpl.lines.'.($i + 1)) <tr><td colspan="6" class="text-danger small py-0">{{ $message }}</td></tr> @enderror
                        @endforeach
                    </tbody>
                </table>
                @error('tpl.lines') <div class="text-danger small mb-2">{{ $message }}</div> @enderror
                <button type="button" class="btn btn-sm btn-light" wire:click="addLine">+ Baris</button>
                <hr>
                <button class="btn btn-primary" wire:click="saveTemplate" wire:loading.attr="disabled">Simpan Template</button>
                <button class="btn btn-light" wire:click="cancel">Batal</button>
            </div>
        </div>
    @elseif ($mode === 'schedule')
        <div class="card">
            <div class="card-header"><h5 class="card-title">Jadwal Jurnal Berulang</h5></div>
            <div class="card-body">
                <p class="text-muted small">Pada tanggal yang ditentukan setiap bulan, sistem membuat <strong>draft</strong> jurnal dari template.
                    Draft tetap harus diajukan dan disetujui (maker-checker). Jadwal di periode yang belum dibuka menunggu sampai Tutup Bulan membuka periodenya.</p>
                <div class="row">
                    <div class="col-md-4 form-group">
                        <label>Template</label>
                        <select class="form-control" wire:model="rec.template_id">
                            <option value="">- pilih -</option>
                            @foreach ($templates->where('is_active', true) as $t)
                                <option value="{{ $t->id }}">{{ $t->name }}</option>
                            @endforeach
                        </select>
                        @error('rec.template_id') <span class="text-danger small">{{ $message }}</span> @enderror
                    </div>
                    <div class="col-md-2 form-group">
                        <label>Setiap tanggal</label>
                        <input type="number" min="1" max="28" class="form-control" wire:model="rec.day_of_month">
                        @error('rec.day_of_month') <span class="text-danger small">{{ $message }}</span> @enderror
                    </div>
                    <div class="col-md-3 form-group">
                        <label>Mulai</label>
                        <input type="date" class="form-control" wire:model="rec.start_date">
                        @error('rec.start_date') <span class="text-danger small">{{ $message }}</span> @enderror
                    </div>
                    <div class="col-md-3 form-group">
                        <label>Selesai (opsional)</label>
                        <input type="date" class="form-control" wire:model="rec.end_date">
                        @error('rec.end_date') <span class="text-danger small">{{ $message }}</span> @enderror
                    </div>
                </div>
                <button class="btn btn-primary" wire:click="saveSchedule" wire:loading.attr="disabled">Simpan Jadwal</button>
                <button class="btn btn-light" wire:click="cancel">Batal</button>
            </div>
        </div>
    @else
        <div class="card">
            <div class="card-header header-elements-inline">
                <h5 class="card-title">Template Jurnal</h5>
                @can('createjournal_template')
                    <button class="btn btn-sm btn-primary" wire:click="newTemplate">+ Template</button>
                @endcan
            </div>
            <div class="card-body">
                @if ($draftFor)
                    <div class="alert alert-info form-inline">
                        <span class="mr-2">Buat draft dari template <strong>{{ $templates->firstWhere('id', $draftFor)?->name }}</strong>, tanggal</span>
                        <input type="date" class="form-control form-control-sm mr-2" wire:model="draftDate">
                        <button class="btn btn-sm btn-primary mr-1" wire:click="createDraft">Buat Draft</button>
                        <button class="btn btn-sm btn-light" wire:click="cancel">Batal</button>
                        @error('date') <span class="text-danger small ml-2">{{ $message }}</span> @enderror
                    </div>
                @endif
                <table class="table table-sm table-hover">
                    <thead class="thead-light"><tr><th>Nama</th><th>Tipe</th><th class="text-right">Baris</th><th class="text-right">Nilai</th><th>Status</th><th></th></tr></thead>
                    <tbody>
                        @forelse ($templates as $t)
                            <tr>
                                <td>{{ $t->name }}<div class="small text-muted">{{ $t->description }}</div></td>
                                <td>{{ $t->trans_code }}</td>
                                <td class="text-right">{{ $t->lines }}</td>
                                <td class="text-right">{{ number_format((float) $t->total, 2, ',', '.') }}</td>
                                <td><span class="badge badge-{{ $t->is_active ? 'success' : 'secondary' }}">{{ $t->is_active ? 'aktif' : 'nonaktif' }}</span></td>
                                <td class="text-nowrap text-right">
                                    @can('createjournal_transaction')
                                        @if ($t->is_active)
                                            <button class="btn btn-sm btn-light" wire:click="askDraft({{ $t->id }})">Buat Draft</button>
                                        @endif
                                    @endcan
                                    @can('createjournal_template')
                                        @if ($t->is_active)
                                            <button class="btn btn-sm btn-light" wire:click="newSchedule({{ $t->id }})">Jadwalkan</button>
                                        @endif
                                        <button class="btn btn-sm btn-light" wire:click="editTemplate({{ $t->id }})">Ubah</button>
                                        <button class="btn btn-sm btn-light" wire:click="toggleTemplate({{ $t->id }})">{{ $t->is_active ? 'Nonaktifkan' : 'Aktifkan' }}</button>
                                        <button class="btn btn-sm btn-link text-danger" wire:click="deleteTemplate({{ $t->id }})" wire:confirm="Hapus template {{ $t->name }}?">Hapus</button>
                                    @endcan
                                </td>
                            </tr>
                        @empty
                            <tr><td colspan="6" class="text-center text-muted py-3">Belum ada template.</td></tr>
                        @endforelse
                    </tbody>
                </table>
            </div>
        </div>

        <div class="card">
            <div class="card-header header-elements-inline">
                <h5 class="card-title">Jadwal Jurnal Berulang</h5>
                @can('createjournal_template')
                    <div>
                        <button class="btn btn-sm btn-light" wire:click="runNow" wire:loading.attr="disabled" title="Biasanya berjalan otomatis setiap jam">Proses jadwal sekarang</button>
                        <button class="btn btn-sm btn-primary" wire:click="newSchedule">+ Jadwal</button>
                    </div>
                @endcan
            </div>
            <div class="card-body">
                <table class="table table-sm">
                    <thead class="thead-light"><tr><th>Template</th><th>Tanggal</th><th>Mulai</th><th>Selesai</th><th>Berikutnya</th><th>Terakhir</th><th>Status</th><th></th></tr></thead>
                    <tbody>
                        @forelse ($schedules as $s)
                            <tr>
                                <td>{{ $s->template_name }}</td>
                                <td>setiap tgl {{ $s->day_of_month }}</td>
                                <td>{{ $s->start_date }}</td>
                                <td>{{ $s->end_date ?? '-' }}</td>
                                <td>{{ $s->next_run }}</td>
                                <td class="small">
                                    @if ($s->last_trans_no)
                                        <a href="{{ url('journal/'.rawurlencode($s->last_trans_no)) }}">{{ $s->last_trans_no }}</a>
                                    @endif
                                    @if ($s->last_error) <div class="text-danger">{{ $s->last_error }}</div> @endif
                                </td>
                                <td><span class="badge badge-{{ $s->is_active ? 'success' : 'secondary' }}">{{ $s->is_active ? 'aktif' : 'berhenti' }}</span></td>
                                <td class="text-nowrap text-right">
                                    @can('createjournal_template')
                                        <button class="btn btn-sm btn-light" wire:click="toggleSchedule({{ $s->id }})">{{ $s->is_active ? 'Hentikan' : 'Aktifkan' }}</button>
                                        <button class="btn btn-sm btn-link text-danger" wire:click="deleteSchedule({{ $s->id }})" wire:confirm="Hapus jadwal ini?">Hapus</button>
                                    @endcan
                                </td>
                            </tr>
                        @empty
                            <tr><td colspan="8" class="text-center text-muted py-3">Belum ada jadwal.</td></tr>
                        @endforelse
                    </tbody>
                </table>

                <h6 class="mt-3">Riwayat proses (30 terakhir)</h6>
                <table class="table table-sm">
                    <thead class="thead-light"><tr><th>Tanggal</th><th>Template</th><th>Status</th><th>Jurnal</th><th>Pesan</th><th></th></tr></thead>
                    <tbody>
                        @forelse ($runs as $r)
                            <tr>
                                <td>{{ $r->run_date }}</td>
                                <td>{{ $r->template_name }}</td>
                                <td><span class="badge badge-{{ $r->status === 'created' ? 'success' : ($r->status === 'failed' ? 'danger' : 'secondary') }}">{{ $r->status }}</span></td>
                                <td>
                                    @if ($r->trans_no)
                                        <a href="{{ url('journal/'.rawurlencode($r->trans_no)) }}">{{ $r->trans_no }}</a>
                                    @else - @endif
                                </td>
                                <td class="small">{{ $r->message }}</td>
                                <td class="text-nowrap text-right">
                                    @if ($r->status === 'failed')
                                        @can('createjournal_template')
                                            <button class="btn btn-sm btn-light" wire:click="retryRun({{ $r->recurring_id }}, '{{ $r->run_date }}')">Coba lagi</button>
                                            <button class="btn btn-sm btn-link" wire:click="skipRun({{ $r->recurring_id }}, '{{ $r->run_date }}')" wire:confirm="Tandai dilewati (misalnya sudah dijurnal manual)?">Lewati</button>
                                        @endcan
                                    @endif
                                </td>
                            </tr>
                        @empty
                            <tr><td colspan="6" class="text-center text-muted py-3">Belum ada proses.</td></tr>
                        @endforelse
                    </tbody>
                </table>
            </div>
        </div>
    @endif
</div>
