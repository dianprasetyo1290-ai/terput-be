<div>
    <div id="modal_default" class="modal fade @if ($isShow) show @endif" tabindex="-1"
        @if ($isShow) style="display: block;" @else style="display: none;" aria-hidden="true" @endif>
        <div class="modal-dialog">
            <div class="modal-content">
                <div class="modal-header">
                    <h5 class="modal-title">Report Final</h5>
                    <button type="button" class="close" data-dismiss="modal" wire:click='showModal'>×</button>
                </div>

                <div class="modal-body">
                    <div class="form-row">
                        <div class="form-group col-md-6">
                            <label for="accountCode">Tahun</label>
                            <div wire:ignore>
                                <select class="form-control select-search2 year @error('year') is-invalid  @enderror"
                                    data-fouc wire:model.live="year">
                                    <option value="">{{ __('Pilih') }}</option>
                                    @foreach ($tahun as $p)
                                        <option value="{{ $p['tahun'] }}">
                                            {{ $p['tahun'] }}
                                        </option>
                                    @endforeach
                                </select>
                            </div>
                            @error('year')
                                <span class="form-text text-danger">
                                    {{ $errors->first('year') }}
                                </span>
                            @enderror
                        </div>
                        <div class="form-group col-md-6">
                            <label for="accountCode">Periode</label>
                            <div wire:ignore>
                                <select
                                    class="form-control select-search2 periode @error('periode') is-invalid  @enderror"
                                    data-fouc wire:model.live="periode">
                                    <option value="">{{ __('Pilih') }}</option>
                                    @foreach ($period as $p)
                                        <option value="{{ $p['p'] }}">
                                            {{ $p['p'] }} [{{ getMonthName($p['no']) }}]
                                        </option>
                                    @endforeach
                                </select>
                            </div>
                            @error('periode')
                                <span class="form-text text-danger">
                                    {{ $errors->first('periode') }}
                                </span>
                            @enderror
                        </div>
                        <div class="form-group col-md-6">
                            <label for="accountCode">Division</label>
                            <div wire:ignore>
                                <select
                                    class="form-control select-search2 div_id @error('div_id') is-invalid  @enderror"
                                    data-fouc wire:model.live="div_id">
                                    <option value="">{{ __('Pilih') }}</option>
                                    @foreach ($div as $d)
                                        <option value="{{ $d['div_code'] }}">
                                            {{ $d['div_code'] }} [{{ $d['div_name'] }}]
                                        </option>
                                    @endforeach
                                </select>
                            </div>
                            @error('div_id')
                                <span class="form-text text-danger">
                                    {{ $errors->first('div_id') }}
                                </span>
                            @enderror
                        </div>
                    </div>
                </div>

                <div class="modal-footer">
                    <button type="button" class="btn btn-link" data-dismiss="modal"
                        wire:click='showModal'>Close</button>
                    <div wire:loading.remove wire:target='showReport'>
                        <button type="button" class="btn bg-primary" wire:click="showReport">Show</button>
                    </div>
                    <div wire:loading wire:target="showReport">
                        <button type="button" class="btn bg-slate-600" disabled>Showing Report...<i
                                class="icon-spinner2 spinner"></i></button>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
