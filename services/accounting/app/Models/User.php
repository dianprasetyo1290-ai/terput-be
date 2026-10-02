<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\SoftDeletes;
use Illuminate\Foundation\Auth\User as Authenticatable;
use Illuminate\Notifications\Notifiable;
use Laravel\Fortify\TwoFactorAuthenticatable;
use Laravel\Jetstream\HasProfilePhoto;
use Laravel\Sanctum\HasApiTokens;
use Spatie\Permission\Traits\HasRoles;
use Illuminate\Support\Str;

class User extends Authenticatable
{
    use HasApiTokens, HasFactory, HasProfilePhoto, Notifiable, TwoFactorAuthenticatable, HasRoles, SoftDeletes;

    protected $table = 'mst_user';

    // Set the primary key column
    protected $primaryKey = 'user_id';

    // If the primary key is not an incrementing integer
    public $incrementing = false;

    // Set the data type of the primary key
    protected $keyType = 'string';

    /**
     * The attributes that are mass assignable.
     *
     * @var array<int, string>
     */
    protected $fillable = ['user_name', 'user_access', 'email', 'password', 'current_team_id', 'profile_photo_path', 'actived'];

    /**
     * The attributes that should be hidden for serialization.
     *
     * @var array<int, string>
     */
    protected $hidden = [
        'password',
        'remember_token',
        'two_factor_recovery_codes',
        'two_factor_secret',
    ];

    /**
     * The accessors to append to the model's array form.
     *
     * @var array<int, string>
     */
    protected $appends = [
        'profile_photo_url',
    ];

    /**
     * Get the attributes that should be cast.
     *
     * @return array<string, string>
     */
    protected function casts(): array
    {
        return [
            'email_verified_at' => 'datetime',
            'password' => 'hashed',
        ];
    }


    public static function create(array $attributes = [])
    {
        $model = new static($attributes);
        $model->save();
        return $model;
    }

    protected static function boot()
    {
        parent::boot();

        static::creating(function ($model) {
            // SSO Sekoci sudah mengisi user_id sendiri (= user_id Sekoci); jangan ditimpa.
            if (blank($model->{$model->getKeyName()})) {
                $model->{$model->getKeyName()} = self::generateSequentialId();
            }
        });
    }

    protected static function generateSequentialId()
    {
        // Hanya id numerik ('0001', ...) yang ikut dihitung: user hasil SSO Sekoci
        // memakai user_id Sekoci yang bukan angka, dan (int)-nya akan jadi 0.
        $lastId = (int) self::withTrashed()->pluck('user_id')
            ->filter(fn ($id) => ctype_digit((string) $id))
            ->map(fn ($id) => (int) $id)
            ->max();

        return str_pad($lastId + 1, 4, '0', STR_PAD_LEFT);
    }
}
